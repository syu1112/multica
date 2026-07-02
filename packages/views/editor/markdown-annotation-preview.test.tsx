// @vitest-environment jsdom

import { describe, expect, it, vi, beforeEach } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MarkdownAnnotationPreview } from "./markdown-annotation-preview";

const createCommentMock = vi.hoisted(() => vi.fn());

vi.mock("@multica/core/api", () => ({
  api: {
    createComment: createCommentMock,
  },
}));

vi.mock("sonner", () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

vi.mock("../i18n", () => ({
  useT: () => ({
    t: (sel: (s: any) => string, vars?: Record<string, unknown>) => {
      const value = sel({
        annotation: {
          count: `批注 ${vars?.count ?? 0}`,
          clear: "清空",
          send_to_comments: "发送到评论区",
          reply_to_comment: "回复该评论",
          add: "添加备注",
          note_placeholder: "输入备注",
          cancel: "取消",
          save: "保存备注",
          list_title: "本次批注",
          sent: "已发送到评论区",
          replied: "已回复该评论",
          send_failed: "发送失败",
          reply_failed: "回复失败",
          empty_note: "请输入备注内容。",
        },
      });
      return value;
    },
  }),
}));

function selectText(node: Node, start: number, end: number) {
  const range = document.createRange();
  range.setStart(node, start);
  range.setEnd(node, end);
  const selection = window.getSelection();
  selection?.removeAllRanges();
  selection?.addRange(range);
}

describe("MarkdownAnnotationPreview", () => {
  beforeEach(() => {
    cleanup();
    createCommentMock.mockReset();
  });

  it("renders common Markdown syntax as formatted preview HTML", () => {
    const { container } = render(
      <MarkdownAnnotationPreview
        attachmentId="att-1"
        filename="README.md"
        content={[
          "# Title",
          "",
          "**bold** and *italic* and [docs](https://example.com)",
          "",
          "- [x] done",
          "- [ ] todo",
          "",
          "| A | B |",
          "| - | - |",
          "| 1 | 2 |",
          "",
          "> quote",
          "",
          "`code`",
          "",
          "---",
        ].join("\n")}
      />,
    );

    expect(container.querySelector("h1")?.textContent).toBe("Title");
    expect(container.querySelector("strong")?.textContent).toBe("bold");
    expect(container.querySelector("em")?.textContent).toBe("italic");
    expect(container.querySelector("a")?.getAttribute("href")).toBe("https://example.com");
    expect(container.querySelectorAll('input[type="checkbox"]')).toHaveLength(2);
    expect(container.querySelector('input[type="checkbox"]')?.hasAttribute("checked")).toBe(true);
    expect(container.querySelector(".tableWrapper table")).not.toBeNull();
    expect(container.querySelector("blockquote")?.textContent).toContain("quote");
    expect(container.querySelector("code")?.textContent).toBe("code");
    expect(container.querySelector("hr")).not.toBeNull();
    expect(container.querySelector("[data-md-start][data-md-end]")).not.toBeNull();
  });

  it("does not render annotation controls without an issue id", () => {
    render(
      <MarkdownAnnotationPreview
        attachmentId="att-1"
        filename="README.md"
        content="hello world"
      />,
    );

    expect(screen.queryByText("发送到评论区")).toBeNull();
  });

  it("maps annotations on formatted Markdown text back to source positions", async () => {
    createCommentMock.mockResolvedValueOnce({ id: "comment-1" });
    const user = userEvent.setup();
    render(
      <MarkdownAnnotationPreview
        attachmentId="att-1"
        filename="README.md"
        issueId="issue-1"
        content="**bold** text"
      />,
    );

    const bold = screen.getByText("bold").firstChild;
    if (!bold) throw new Error("missing bold text node");
    selectText(bold, 0, 4);
    fireEvent.mouseUp(screen.getByTestId("markdown-annotation-source"));

    await user.type(screen.getByPlaceholderText("输入备注"), "formatted note");
    await user.click(screen.getByText("保存备注"));
    await user.click(screen.getByText("发送到评论区"));

    await waitFor(() => {
      expect(createCommentMock).toHaveBeenCalledTimes(1);
    });
    const body = createCommentMock.mock.calls[0]?.[1] as string;
    expect(body).toContain("`README.md:L1:C3-L1:C6`");
    expect(body).toContain("备注：formatted note");
  });

  it("replies to the source comment when a parent comment id is provided", async () => {
    createCommentMock.mockResolvedValueOnce({ id: "comment-reply-1" });
    const user = userEvent.setup();
    render(
      <MarkdownAnnotationPreview
        attachmentId="att-1"
        filename="README.md"
        issueId="issue-1"
        replyToCommentId="comment-parent-1"
        content="hello world"
      />,
    );

    expect(screen.getByText("回复该评论")).toBeTruthy();

    const text = screen.getByText("hello world").firstChild;
    if (!text) throw new Error("missing text node");
    selectText(text, 0, 5);
    fireEvent.mouseUp(screen.getByTestId("markdown-annotation-source"));

    await user.type(screen.getByPlaceholderText("输入备注"), "reply note");
    await user.click(screen.getByText("保存备注"));
    await user.click(screen.getByText("回复该评论"));

    await waitFor(() => {
      expect(createCommentMock).toHaveBeenCalledTimes(1);
    });
    expect(createCommentMock.mock.calls[0]?.[0]).toBe("issue-1");
    expect(createCommentMock.mock.calls[0]?.[2]).toBeUndefined();
    expect(createCommentMock.mock.calls[0]?.[3]).toBe("comment-parent-1");
  });

  it("highlights the add-note panel with a light orange treatment", () => {
    render(
      <MarkdownAnnotationPreview
        attachmentId="att-1"
        filename="README.md"
        issueId="issue-1"
        content="hello world"
      />,
    );

    const text = screen.getByText("hello world").firstChild;
    if (!text) throw new Error("missing text node");
    selectText(text, 0, 5);
    fireEvent.mouseUp(screen.getByTestId("markdown-annotation-source"));

    const panel = screen.getByTestId("markdown-annotation-note-panel");
    expect(panel.className).toContain("bg-orange-50");
    expect(panel.className).toContain("border-orange-200");
    expect(screen.getByPlaceholderText("输入备注").className).toContain("bg-orange-100");
    expect(screen.getByText("取消").className).toContain("bg-orange-200");
    expect(screen.getByText("保存备注").className).toContain("bg-orange-200");
  });

  it("adds multiple annotations and sends them as one issue comment", async () => {
    createCommentMock.mockResolvedValueOnce({ id: "comment-1" });
    const user = userEvent.setup();
    render(
      <MarkdownAnnotationPreview
        attachmentId="att-1"
        filename="README.md"
        issueId="issue-1"
        content={"hello world\nsecond line"}
      />,
    );

    const world = screen.getByText("hello world").firstChild;
    if (!world) throw new Error("missing text node");
    selectText(world, 6, 11);
    fireEvent.mouseUp(screen.getByTestId("markdown-annotation-source"));
    expect(screen.getByText("添加备注")).toBeTruthy();

    await user.type(screen.getByPlaceholderText("输入备注"), "explain this");
    await user.click(screen.getByText("保存备注"));
    expect(screen.getByText("批注 1")).toBeTruthy();
    expect(screen.getByText(/README\.md:L1:C7-L1:C11/)).toBeTruthy();
    const listPanel = screen.getByTestId("markdown-annotation-list-panel");
    expect(listPanel.className).toContain("bg-orange-50");
    expect(listPanel.className).toContain("border-orange-200");
    const firstAnnotationItem = listPanel.querySelector("li");
    expect(firstAnnotationItem?.className).toContain("bg-orange-100");
    expect(firstAnnotationItem?.className).toContain("border-orange-200");

    const second = screen.getByText("second line").firstChild;
    if (!second) throw new Error("missing second text node");
    selectText(second, 0, 6);
    fireEvent.mouseUp(screen.getByTestId("markdown-annotation-source"));
    await user.type(screen.getByPlaceholderText("输入备注"), "second note");
    await user.click(screen.getByText("保存备注"));
    expect(screen.getByText("批注 2")).toBeTruthy();

    await user.click(screen.getByText("发送到评论区"));

    await waitFor(() => {
      expect(createCommentMock).toHaveBeenCalledTimes(1);
    });
    const call = createCommentMock.mock.calls[0];
    expect(call).toBeDefined();
    expect(call?.[0]).toBe("issue-1");
    const body = call?.[1] as string;
    expect(body).toContain("Markdown 批注：README.md");
    expect(body).toContain("`README.md:L1:C7-L1:C11`");
    expect(body).toContain("备注：explain this");
    expect(body).toContain("`README.md:L2:C1-L2:C6`");
    expect(body).toContain("备注：second note");
  });
});
