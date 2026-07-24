import { describe, expect, it, vi } from "vitest";
import {
  ScriptedEventStream,
  createScriptedStreamController,
  type StreamStep,
} from "./scriptedStream";

describe("ScriptedEventStream", () => {
  it("delivers message, error, and end in explicit script order", () => {
    const path = "/api/sessions/story-session/stream";
    const script: StreamStep[] = [
      { type: "message", data: { kind: "text_delta", text: "Ready" } },
      { type: "error", error: "fixture disconnect" },
      { type: "end", data: { reason: "complete" } },
    ];
    const controller = createScriptedStreamController({ [path]: script });
    const stream = controller.open(path);
    const order: string[] = [];

    stream.onmessage = (event) => order.push(`message:${event.data}`);
    stream.onerror = () => order.push("error");
    stream.addEventListener("end", () => order.push("end"));

    expect(controller.playAll(stream)).toEqual(script);
    expect(order).toEqual([
      'message:{"kind":"text_delta","text":"Ready"}',
      "error",
      "end",
    ]);
    expect(controller.progress(stream)).toEqual({
      path,
      cursor: 3,
      total: 3,
      closed: false,
    });
  });

  it("never dispatches or advances after close", () => {
    const path = "/api/runs/story-run/stream";
    const controller = createScriptedStreamController({
      [path]: [{ type: "message", data: "line one" }],
    });
    const stream = controller.open(path);
    const listener = vi.fn();
    stream.onmessage = listener;

    stream.close();

    expect(stream.emitMessage("direct")).toBe(false);
    expect(stream.emitError()).toBe(false);
    expect(stream.emitEnd()).toBe(false);
    expect(controller.next(stream)).toBeNull();
    expect(controller.progress(stream)?.cursor).toBe(0);
    expect(listener).not.toHaveBeenCalled();
  });

  it("finishes the current dispatch when a listener closes the source", () => {
    const stream = new ScriptedEventStream("/api/direct");
    const order: string[] = [];
    stream.onmessage = () => {
      order.push("property");
      stream.close();
    };
    stream.addEventListener("message", () => order.push("listener"));

    expect(stream.emitMessage("one")).toBe(true);
    expect(order).toEqual(["property", "listener"]);
    expect(stream.emitMessage("two")).toBe(false);
  });

  it("isolates cursors for two streams opened on the same path", () => {
    const path = "/api/sessions/story-session/stream";
    const controller = createScriptedStreamController({
      [path]: [
        { type: "message", data: "first" },
        { type: "message", data: "second" },
      ],
    });
    const first = controller.open(path);
    const second = controller.open(path);

    expect(controller.next(first)).toEqual({ type: "message", data: "first" });
    expect(controller.next(first)).toEqual({ type: "message", data: "second" });
    expect(controller.next(second)).toEqual({ type: "message", data: "first" });
    expect(controller.progress(first)?.cursor).toBe(2);
    expect(controller.progress(second)?.cursor).toBe(1);
  });

  it("reset closes all existing streams and forgets their state", () => {
    const controller = createScriptedStreamController();
    const stream = controller.open("/api/runs/story-run/stream");

    controller.reset();

    expect(stream.closed).toBe(true);
    expect(controller.streams()).toEqual([]);
  });

  it("accepts direct stream use without a controller", () => {
    const stream = new ScriptedEventStream("/api/direct");
    const message = vi.fn();
    stream.onmessage = message;

    expect(stream.emitMessage("hello")).toBe(true);
    expect(message.mock.calls[0][0].data).toBe("hello");
  });
});
