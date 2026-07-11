import { Component, type ReactNode } from "react";

// ErrorBoundary keeps a single component's render error from unmounting the
// whole cockpit to a blank screen. It resets when `resetKey` changes (e.g. on
// navigating to a different session) so a bad view doesn't stick.
interface Props {
  resetKey: string;
  children: ReactNode;
}
interface State {
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidUpdate(prev: Props) {
    if (prev.resetKey !== this.props.resetKey && this.state.error) {
      this.setState({ error: null });
    }
  }

  render() {
    if (this.state.error) {
      return (
        <div className="m-auto text-center text-dim max-w-[340px]">
          <div className="text-[34px] mb-[6px]">⚠️</div>
          <div>This view hit a render error.</div>
          <pre style={{ fontSize: 12, color: "var(--red)", whiteSpace: "pre-wrap", marginTop: 8 }}>
            {String(this.state.error.message || this.state.error)}
          </pre>
          <button style={{ marginTop: 10 }} onClick={() => this.setState({ error: null })}>
            Retry
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}
