package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ralphite/agentrunner/internal/provider"
)

// defaultHelperModel is the model the composer-helper commands (dictate,
// optimize) use when the caller doesn't override it. Gemini is the primary
// provider (DESIGN 决策 #1) and flash is fast/cheap enough for a one-shot
// transcription or rewrite.
const (
	defaultHelperProvider = "gemini"
	defaultHelperModel    = "gemini-flash-latest"
	// defaultDictateMaxBytes caps an uploaded recording. Gemini inline_data
	// rides in the request body (~20MB ceiling); 20MB of audio is minutes of
	// speech — far more than a composer dictation — so it doubles as an
	// abuse guard.
	defaultDictateMaxBytes = 20 << 20
)

// dictateOptions carries everything runDictate needs; factored for tests so
// the provider call can be driven by an injected factory.
type dictateOptions struct {
	audioPath string
	model     string
	prov      string
	context   string // optional disambiguation hint (proper nouns, domain, language mix)
	mime      string // optional explicit MIME type; inferred from extension otherwise
	maxBytes  int64
	factory   providerFactory
	stdout    io.Writer
	stderr    io.Writer
}

// dictateCmd transcribes an audio recording to text via the provider
// (INC-56, HANDA-PARITY #18):
//
//	agentrunner dictate recording.wav
//	agentrunner dictate --context "Kubernetes, kubelet, Ralph" note.webm
//
// It is a one-shot provider call — no daemon, no session, no journal. The
// transcript is the ONLY thing printed to stdout so a caller (the webui thin
// shell) can capture it cleanly; diagnostics go to stderr. This is a composer
// text convenience: the audio is transcribed to text that then enters the
// composer as an ordinary prompt. The agent loop never sees an audio part.
func dictateCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("dictate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	model := fs.String("model", defaultHelperModel, "model id")
	prov := fs.String("provider", defaultHelperProvider, "provider name")
	contextHint := fs.String("context", "", "optional context to disambiguate proper nouns / mixed-language terms")
	mime := fs.String("mime", "", "audio MIME type (inferred from the file extension otherwise)")
	maxBytes := fs.Int64("max-bytes", defaultDictateMaxBytes, "reject audio larger than this many bytes")
	if ok, code := parseFlags(fs, args); !ok {
		return code
	}
	rest := fs.Args()
	if len(rest) != 1 || strings.TrimSpace(rest[0]) == "" {
		fmt.Fprintln(stderr, "usage: agentrunner dictate [--context \"...\"] [--model id] <audio-file>")
		return ExitUsage
	}
	return runDictate(dictateOptions{
		audioPath: rest[0],
		model:     *model,
		prov:      *prov,
		context:   *contextHint,
		mime:      *mime,
		maxBytes:  *maxBytes,
		factory:   defaultProviderFactory,
		stdout:    stdout,
		stderr:    stderr,
	})
}

func runDictate(opts dictateOptions) int {
	loadDotEnv(".env")
	if opts.factory == nil {
		opts.factory = defaultProviderFactory
	}
	if opts.maxBytes <= 0 {
		opts.maxBytes = defaultDictateMaxBytes
	}

	info, err := os.Stat(opts.audioPath)
	if err != nil {
		fmt.Fprintf(opts.stderr, "agentrunner: %v\n", err)
		return ExitUsage
	}
	if info.IsDir() {
		fmt.Fprintf(opts.stderr, "agentrunner: %s is a directory, not an audio file\n", opts.audioPath)
		return ExitUsage
	}
	if info.Size() > opts.maxBytes {
		fmt.Fprintf(opts.stderr, "agentrunner: audio is %d bytes, over the %d-byte limit (raise --max-bytes to allow it)\n", info.Size(), opts.maxBytes)
		return ExitUsage
	}
	mime := opts.mime
	if mime == "" {
		mime = audioMIME(opts.audioPath)
	}
	if mime == "" {
		fmt.Fprintf(opts.stderr, "agentrunner: can't infer an audio MIME type from %q — pass --mime (e.g. audio/wav)\n", filepath.Ext(opts.audioPath))
		return ExitUsage
	}
	data, err := os.ReadFile(opts.audioPath)
	if err != nil {
		fmt.Fprintf(opts.stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	if len(data) == 0 {
		fmt.Fprintln(opts.stderr, "agentrunner: audio file is empty")
		return ExitUsage
	}

	ctx := context.Background()
	prov, err := opts.factory(ctx, opts.prov)
	if err != nil {
		fmt.Fprintln(opts.stderr, err)
		if errors.Is(err, errUnknownProvider) {
			return ExitUsage
		}
		return ExitRun
	}

	req := provider.CompleteRequest{
		Model:     opts.model,
		MaxTokens: 4096,
		System:    dictateSystemPrompt(opts.context),
		Messages: []provider.Message{{Role: provider.RoleUser, Parts: []provider.Part{
			{Kind: provider.PartText, Text: "Transcribe this audio recording."},
			{Kind: provider.PartAudio, MediaType: mime, Data: data},
		}}},
	}
	turn, err := provider.CollectTurnStreaming(prov.Complete(ctx, req), func(string) {})
	if err != nil {
		fmt.Fprintf(opts.stderr, "agentrunner: dictate failed: %v\n", err)
		return ExitRun
	}
	transcript := strings.TrimSpace(assistantMessageText(turn.Message))
	if transcript == "" {
		fmt.Fprintln(opts.stderr, "agentrunner: the model returned no transcript")
		return ExitRun
	}
	fmt.Fprintln(opts.stdout, transcript)
	return ExitOK
}

// dictateSystemPrompt builds the transcription instruction, folding in the
// caller's context so proper nouns and mixed-language terms are spelled right.
func dictateSystemPrompt(contextHint string) string {
	var b strings.Builder
	b.WriteString("You transcribe speech to text. Output ONLY the verbatim transcript of the words spoken in the audio — ")
	b.WriteString("no preamble, no translation, no commentary, no quotation marks, no markdown. ")
	b.WriteString("The speaker may mix languages (for example Chinese and English); keep each word in the language it was spoken. ")
	b.WriteString("Preserve proper nouns, technical terms, and code identifiers exactly. ")
	b.WriteString("If the audio contains no discernible speech, output nothing.")
	if h := strings.TrimSpace(contextHint); h != "" {
		b.WriteString("\n\nContext to disambiguate names and terms (do not transcribe this text, use it only to spell things correctly):\n")
		b.WriteString(h)
	}
	return b.String()
}

// audioMIME infers a Gemini-acceptable audio MIME type from a filename's
// extension. Empty means "unknown — ask the caller for --mime".
func audioMIME(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".wav":
		return "audio/wav"
	case ".mp3":
		return "audio/mp3"
	case ".aiff", ".aif":
		return "audio/aiff"
	case ".aac":
		return "audio/aac"
	case ".ogg", ".oga", ".opus":
		return "audio/ogg"
	case ".flac":
		return "audio/flac"
	case ".m4a":
		return "audio/mp4"
	case ".webm":
		return "audio/webm"
	default:
		return ""
	}
}

// assistantMessageText concatenates the text parts of a message (tool-call
// parts have no text). The helper commands issue a tool-less turn, so the
// answer is entirely text parts.
func assistantMessageText(m provider.Message) string {
	var b strings.Builder
	for _, p := range m.Parts {
		if p.Kind == provider.PartText {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}
