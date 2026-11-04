const escapeRegExp = (text: string): string => text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

export const parseNodeId = (id: string, viewId: string, diagramType: string): string | null => {
  const prefix = `${viewId}-${diagramType}-`;
  if (!id.startsWith(prefix)) {
    return null;
  }
  const rest = id.slice(prefix.length);
  // Strip the trailing -<counter> segment added by mermaid's renderer.
  const withoutCounter = rest.replace(/-\d+$/, '');
  if (!withoutCounter || withoutCounter === rest) {
    return null;
  }
  return withoutCounter;
};

export const findDefinitionLine = (code: string, nodeName: string): number | null => {
  const pattern = new RegExp(`(^|\\W)${escapeRegExp(nodeName)}([\\[({"']|\\s*$)`, 'm');
  const match = pattern.exec(code);
  if (!match) {
    return null;
  }
  return code.slice(0, match.index).split('\n').length;
};

interface BracketVariant {
  close: string;
  open: string;
}

// Mermaid flowchart label shapes: A[label], A(label), A{label}.
const bracketVariants: BracketVariant[] = [
  { close: ']', open: '[' },
  { close: ')', open: '(' },
  { close: '}', open: '{' }
];

/**
 * Replaces the payload inside the bracketed label of the first definition of
 * `nodeName`, keeping the node name, brackets and any surrounding quotes.
 * Falls through (returns null) when no such definition exists.
 */
const replaceBracketLabel = (
  code: string,
  nodeName: string,
  newLabel: string,
  variant: BracketVariant
): string | null => {
  const { open, close } = variant;
  // Quoted payloads may contain the closing bracket; bare ones may not.
  const inner = close === ']' ? '[^"\'\\]]' : `[^"'${close}]`;
  const pattern = new RegExp(
    `(^|\\W)(${escapeRegExp(nodeName)})${escapeRegExp(open)}` +
      `(?:"([^"]*)"|'([^']*)'|(${inner}*))` +
      escapeRegExp(close)
  );
  const match = pattern.exec(code);
  if (!match) {
    return null;
  }
  let payload: string;
  if (match[3] !== undefined) {
    payload = `"${newLabel}"`;
  } else if (match[4] !== undefined) {
    payload = `'${newLabel}'`;
  } else {
    payload = newLabel;
  }
  const start = match.index + match[1].length;
  const end = match.index + match[0].length;
  return code.slice(0, start) + match[2] + open + payload + close + code.slice(end);
};

/** Replaces the alias of `participant <nodeName> as <alias>`. */
const replaceParticipantAlias = (
  code: string,
  nodeName: string,
  newLabel: string
): string | null => {
  const pattern = new RegExp(`^(\\s*participant\\s+${escapeRegExp(nodeName)}\\s+as\\s+)(.*)$`, 'm');
  const match = pattern.exec(code);
  if (!match) {
    return null;
  }
  return (
    code.slice(0, match.index) + match[1] + newLabel + code.slice(match.index + match[0].length)
  );
};

/**
 * Replaces a bare sequence actor declaration (`Old->>B: hi`), where the actor
 * name doubles as the displayed label. The arrow must directly follow the name
 * so flowchart edges like `A --> B` are left untouched.
 */
const replaceBareActor = (code: string, nodeName: string, newLabel: string): string | null => {
  const pattern = new RegExp(`^(\\s*)(${escapeRegExp(nodeName)})(?:->>|-->>|-->|->|--)`, 'm');
  const match = pattern.exec(code);
  if (!match) {
    return null;
  }
  const start = match.index + match[1].length;
  const end = start + match[2].length;
  return code.slice(0, start) + newLabel + code.slice(end);
};

export const replaceNodeLabel = (
  code: string,
  nodeName: string,
  newLabel: string
): string | null => {
  for (const variant of bracketVariants) {
    const replaced = replaceBracketLabel(code, nodeName, newLabel, variant);
    if (replaced !== null) {
      return replaced;
    }
  }
  const participant = replaceParticipantAlias(code, nodeName, newLabel);
  if (participant !== null) {
    return participant;
  }
  return replaceBareActor(code, nodeName, newLabel);
};
