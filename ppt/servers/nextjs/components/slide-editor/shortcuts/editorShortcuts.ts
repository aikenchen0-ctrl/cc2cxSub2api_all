export type EditorShortcutKey =
  | "Mod"
  | "Shift"
  | "Alt"
  | "Click"
  | "Backspace"
  | "Delete"
  | "ArrowLeft"
  | "ArrowRight"
  | "ArrowUp"
  | "ArrowDown"
  | "C"
  | "G"
  | "J"
  | "K"
  | "V"
  | "Y"
  | "Z"
  | "?";

export type EditorShortcut = {
  id:
    | "multi-select"
    | "group"
    | "delete"
    | "copy"
    | "paste"
    | "undo"
    | "redo"
    | "bring-forward"
    | "bring-to-front"
    | "send-backward"
    | "send-to-back"
    | "previous-slide"
    | "next-slide"
    | "shortcut-help";
  label: string;
  description: string;
  chords: EditorShortcutKey[][];
};

export type EditorShortcutSection = {
  id: "selection" | "editing" | "arrange" | "navigation" | "help";
  title: string;
  description: string;
  shortcuts: EditorShortcut[];
};

const ARROW_KEY_LABELS: Partial<Record<EditorShortcutKey, string>> = {
  ArrowLeft: "←",
  ArrowRight: "→",
  ArrowUp: "↑",
  ArrowDown: "↓",
};

export const EDITOR_SHORTCUT_SECTIONS: EditorShortcutSection[] = [
  {
    id: "selection",
    title: "选择",
    description: "在活动的幻灯片上选择和整理对象",
    shortcuts: [
      {
        id: "multi-select",
        label: "添加到选择",
        description: "按住 Shift 键点击以添加或删除对象。",
        chords: [["Shift", "Click"]],
      },
      {
        id: "group",
        label: "组合对象",
        description: "将两个或多个选中的对象合并为一个组。",
        chords: [["Mod", "G"]],
      },
      {
        id: "delete",
        label: "删除选中项",
        description: "从幻灯片中移除选定的对象或对象。",
        chords: [["Backspace"], ["Delete"]],
      },
    ],
  },
  {
    id: "editing",
    title: "编辑",
    description: "幻灯片对象的常用编辑命令。",
    shortcuts: [
      {
        id: "undo",
        label: "撤销",
        description: "撤销在当前幻灯片上所做的最后更改。",
        chords: [["Mod", "Z"]],
      },
      {
        id: "redo",
        label: "重做",
        description: "恢复最近撤销的幻灯片更改。",
        chords: [["Mod", "Shift", "Z"], ["Mod", "Y"]],
      },
      {
        id: "copy",
        label: "复制选中项",
        description: "复制所选对象或对象。",
        chords: [["Mod", "C"]],
      },
      {
        id: "paste",
        label: "粘贴",
        description: "粘贴复制的对象，并带有小的位置偏移。",
        chords: [["Mod", "V"]],
      },
    ],
  },
  {
    id: "arrange",
    title: "排列",
    description: "更改所选对象的图层位置。",
    shortcuts: [
      {
        id: "bring-forward",
        label: "提前",
        description: "将选中的对象向前移动一层。",
        chords: [["Alt", "K"]],
      },
      {
        id: "bring-to-front",
        label: "置于顶层",
        description: "将选中的对象置于其他所有对象的上方。",
        chords: [["Shift", "Alt", "K"]],
      },
      {
        id: "send-backward",
        label: "向后发送",
        description: "将选中的对象向后移动一层。",
        chords: [["Alt", "J"]],
      },
      {
        id: "send-to-back",
        label: "发送到后层",
        description: "将选中的对象置于其他所有对象之后。",
        chords: [["Shift", "Alt", "J"]],
      },
    ],
  },
  {
    id: "navigation",
    title: "幻灯片导航",
    description: "在不离开画布的情况下在幻灯片之间切换。",
    shortcuts: [
      {
        id: "previous-slide",
        label: "上一张",
        description: "打开当前幻灯片之前的幻灯片。",
        chords: [["ArrowLeft"], ["ArrowUp"]],
      },
      {
        id: "next-slide",
        label: "下一张",
        description: "打开当前幻灯片之后的幻灯片。",
        chords: [["ArrowRight"], ["ArrowDown"]],
      },
    ],
  },
  {
    id: "help",
    title: "帮助",
    description: "快速返回到此参考。",
    shortcuts: [
      {
        id: "shortcut-help",
        label: "键盘快捷键",
        description: "打开此键盘快捷键指南。",
        chords: [["?"]],
      },
    ],
  },
];

export function editorShortcutById(id: EditorShortcut["id"]) {
  return EDITOR_SHORTCUT_SECTIONS.flatMap(
    (section) => section.shortcuts,
  ).find((shortcut) => shortcut.id === id);
}

export function shortcutKeyLabel(
  key: EditorShortcutKey,
  applePlatform: boolean,
) {
  const arrowLabel = ARROW_KEY_LABELS[key];
  if (arrowLabel) return arrowLabel;

  if (!applePlatform) {
    if (key === "Mod") return "Ctrl";
    return key;
  }

  switch (key) {
    case "Mod":
      return "⌘";
    case "Shift":
      return "⇧";
    case "Alt":
      return "⌥";
    case "Backspace":
      return "⌫";
    default:
      return key;
  }
}
