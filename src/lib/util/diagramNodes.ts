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
 * Escapes a replacement label for safe insertion into mermaid code: double
 * quotes become the `#quot;` entity. Returns null when the label cannot be
 * represented safely (single quotes, line breaks).
 */
const escapeLabel = (label: string): string | null => {
  if (/['\r\n]/.test(label)) {
    return null;
  }
  return label.replaceAll('"', '#quot;');
};

/**
 * Replaces the payload inside the bracketed label of the first definition of
 * `nodeName`, keeping the node name, brackets and any surrounding quotes.
 * `label` must already be escaped via `escapeLabel`. Returns null when no such
 * definition exists, or when the label contains the closing bracket and would
 * terminate the label early.
 */
const replaceBracketLabel = (
  code: string,
  nodeName: string,
  label: string,
  variant: BracketVariant
): string | null => {
  const { open, close } = variant;
  if (label.includes(close)) {
    // The payload would break out of the bracketed label — not representable.
    return null;
  }
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
    payload = `"${label}"`;
  } else if (match[4] !== undefined) {
    payload = `'${label}'`;
  } else {
    payload = label;
  }
  const start = match.index + match[1].length;
  const end = match.index + match[0].length;
  return code.slice(0, start) + match[2] + open + payload + close + code.slice(end);
};

/** Replaces the alias of `participant <nodeName> as <alias>`. `label` must be escaped. */
const replaceParticipantAlias = (code: string, nodeName: string, label: string): string | null => {
  // `[^\r\n]*` stops before the carriage return so CRLF line endings survive.
  const pattern = new RegExp(
    `^(\\s*participant\\s+${escapeRegExp(nodeName)}\\s+as\\s+)([^\\r\\n]*)$`,
    'm'
  );
  const match = pattern.exec(code);
  if (!match) {
    return null;
  }
  return code.slice(0, match.index) + match[1] + label + code.slice(match.index + match[0].length);
};

/**
 * Replaces a bare sequence actor declaration (`Old->>B: hi`), where the actor
 * name doubles as the displayed label. The arrow must directly follow the name
 * so flowchart edges like `A --> B` are left untouched. `label` must be escaped.
 */
const replaceBareActor = (code: string, nodeName: string, label: string): string | null => {
  const pattern = new RegExp(`^(\\s*)(${escapeRegExp(nodeName)})(?:->>|-->>|-->|->|--)`, 'm');
  const match = pattern.exec(code);
  if (!match) {
    return null;
  }
  const start = match.index + match[1].length;
  const end = start + match[2].length;
  return code.slice(0, start) + label + code.slice(end);
};

const isSequenceDiagram = (diagramType: string): boolean =>
  diagramType === 'sequence' || diagramType === 'sequenceDiagram';

export const replaceNodeLabel = (
  code: string,
  nodeName: string,
  newLabel: string,
  diagramType?: string
): string | null => {
  const label = escapeLabel(newLabel);
  if (label === null) {
    return null;
  }
  for (const variant of bracketVariants) {
    const replaced = replaceBracketLabel(code, nodeName, label, variant);
    if (replaced !== null) {
      return replaced;
    }
  }
  // The participant/bare-actor syntax only exists in sequence diagrams. When
  // the caller supplies a diagramType, trust it; when omitted (legacy callers
  // and tests), fall back to trying every variant.
  if (diagramType !== undefined && !isSequenceDiagram(diagramType)) {
    return null;
  }
  const participant = replaceParticipantAlias(code, nodeName, label);
  if (participant !== null) {
    return participant;
  }
  return replaceBareActor(code, nodeName, label);
};
