import { describe, expect, it } from "vitest";
import { describeMargins, jobActive, jobKind, jobLabel, newRequestId, pickPaper } from "./printInfo";

const papers = [{ id: 9, name: "A4", widthIn: 8.27, heightIn: 11.69 }, { id: 119, name: "4x6 in", widthIn: 4, heightIn: 6 }, { id: 120, name: "5x7 in", widthIn: 5, heightIn: 7 }];

describe("print helpers", () => {
  it("never calls a spooler-only result proof of delivery", () => {
    expect(jobLabel.completed).toMatch(/spooler says so/);
    expect(jobLabel.removed).toBe("Left the queue");
    expect(jobKind("failed")).toBe("err");
    expect(jobKind("unknown")).toBe("warn");
  });
  it("knows which states can still change", () => {
    expect(["submitting", "queued", "printing"].every((s) => jobActive(s as never))).toBe(true);
    expect(["completed", "removed", "failed", "unknown"].some((s) => jobActive(s as never))).toBe(false);
  });
  it("generates a different request id for every action", () => expect(newRequestId()).not.toBe(newRequestId()));
  it("chooses the saved paper, else the matching size in either orientation, else the first", () => {
    expect(pickPaper(papers, "5x7 in")).toBe("5x7 in");
    expect(pickPaper(papers, "gone", [6, 4])).toBe("4x6 in");
    expect(pickPaper(papers, "", undefined)).toBe("A4");
    expect(pickPaper([], "x")).toBe("");
  });
  it("describes margins", () => expect(describeMargins([25.4, 0, 25.4, -0.2])).toBe("left 25.4 mm, top 0.0 mm, right 25.4 mm, bottom 0.0 mm"));
});
