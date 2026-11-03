import { env } from './env';

const SYSTEM_PROMPT = [
  'You are a Mermaid diagram editor.',
  'The user provides the current Mermaid diagram code and an instruction.',
  'Apply the instruction to the diagram and return ONLY the complete updated Mermaid diagram code.',
  'Do not include any explanations, comments about your changes, or markdown code fences.'
].join(' ');

export const aiEnabled = env.aiEnabled;

export const parseAIContent = (raw: string): string => {
  const trimmed = raw.trim();
  const fenceMatch = trimmed.match(/^```(?:mermaid)?\s*([\s\S]*?)```$/);
  return (fenceMatch ? (fenceMatch[1] ?? '') : trimmed).trim();
};

export const editDiagramWithAI = async (
  instruction: string,
  currentCode: string
): Promise<string> => {
  const response = await fetch('/ai/v1/chat/completions', {
    body: JSON.stringify({
      messages: [
        { role: 'system', content: SYSTEM_PROMPT },
        {
          content: `Current diagram code:\n\n\`\`\`\n${currentCode}\n\`\`\`\n\nInstruction: ${instruction}`,
          role: 'user'
        }
      ],
      model: env.aiModel
    }),
    headers: { 'Content-Type': 'application/json' },
    method: 'POST',
    signal: AbortSignal.timeout(60_000)
  });

  if (!response.ok) {
    const errorText = await response.text().catch(() => '');
    throw new Error(`AI request failed (${response.status}): ${errorText.slice(0, 200)}`.trim());
  }

  let data: { choices?: { message?: { content?: string } }[] };
  try {
    data = (await response.json()) as { choices?: { message?: { content?: string } }[] };
  } catch {
    throw new Error('AI returned an invalid (non-JSON) response');
  }
  const content = data.choices?.[0]?.message?.content ?? '';
  if (!content.trim()) {
    throw new Error('AI returned an empty response');
  }
  return parseAIContent(content);
};
