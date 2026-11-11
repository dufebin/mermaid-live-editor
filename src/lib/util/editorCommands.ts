// Bridge for components outside the editor (e.g. the AI edit bar) to run
// editor commands. DesktopEditor registers its Monaco instance on mount;
// until then (or on mobile, where no Monaco exists) calls are no-ops.
interface EditorLike {
  focus: () => void;
  trigger: (source: string, handlerId: string, payload: unknown) => void;
}

let editor: EditorLike | undefined;

export const setEditor = (instance: EditorLike | undefined): void => {
  editor = instance;
};

// Focus the editor and run a built-in command (e.g. 'undo' / 'redo').
export const triggerEditorCommand = (command: 'redo' | 'undo'): void => {
  editor?.focus();
  editor?.trigger('ai-edit-bar', command, null);
};
