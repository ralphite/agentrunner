import { useRef, useState } from "react";
import { AR } from "../api";

export function Composer({
  statusText,
  onSend,
  onError,
}: {
  statusText: string;
  onSend: (text: string, images: string[]) => Promise<void>;
  onError: (msg: string) => void;
}) {
  const [text, setText] = useState("");
  const [images, setImages] = useState<{ path: string; name: string }[]>([]);
  const fileRef = useRef<HTMLInputElement>(null);
  const taRef = useRef<HTMLTextAreaElement>(null);

  const submit = async () => {
    const t = text.trim();
    if (!t) return;
    const imgs = images.map((i) => i.path);
    setText("");
    setImages([]);
    if (taRef.current) taRef.current.style.height = "auto";
    await onSend(t, imgs);
  };

  const onKey = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      submit();
    }
  };

  const pickImage = async (file: File) => {
    try {
      const r = await AR.upload(file);
      setImages((p) => [...p, r]);
    } catch (e: any) {
      onError(e.message);
    }
  };

  return (
    <div className="composer">
      <div className="composer-inner">
        {images.length > 0 && (
          <div className="imgchips">
            {images.map((img, i) => (
              <span
                className="imgchip"
                key={i}
                onClick={() => setImages((p) => p.filter((_, j) => j !== i))}
              >
                📷 {img.name} ✕
              </span>
            ))}
          </div>
        )}
        <div className="box">
          <textarea
            ref={taRef}
            value={text}
            placeholder="给 agent 发消息…(Enter 发送，Shift+Enter 换行)"
            onChange={(e) => {
              setText(e.target.value);
              const el = e.target;
              el.style.height = "auto";
              el.style.height = Math.min(el.scrollHeight, 200) + "px";
            }}
            onKeyDown={onKey}
            rows={1}
          />
          <input
            type="file"
            accept="image/*"
            ref={fileRef}
            style={{ display: "none" }}
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (f) pickImage(f);
              e.target.value = "";
            }}
          />
          <button className="icon-btn ghost" title="附加图片" onClick={() => fileRef.current?.click()}>
            📎
          </button>
          <button className="primary send-btn" onClick={submit} disabled={!text.trim()}>
            发送
          </button>
        </div>
        <div className="statusline">{statusText}</div>
      </div>
    </div>
  );
}
