import type { State } from './types';

export const TID = {
  aiHelpText: 'ai-help-text',
  aiRepairButton: 'ai-repair-button',
  copyMarkdown: 'copy-markdown',
  dashboardPage: 'dashboard-page',
  diagramCard: 'diagram-card',
  diagramDocumentationButton: 'diagram-documentation-button',
  diagramsLink: 'diagrams-link',
  downloadPNG: 'download-PNG',
  downloadSVG: 'download-SVG',
  embedEditLink: 'embed-edit-link',
  embedErrorCard: 'embed-error-card',
  embedFooter: 'embed-footer',
  embedModeToggle: 'embed-mode-toggle',
  embedPreview: 'embed-preview',
  embedRenderError: 'embed-render-error',
  embedSaveLink: 'embed-save-link',
  embedSnippet: 'embed-snippet',
  embedToolbar: 'embed-toolbar',
  errorContainer: 'error-container',
  exportButton: 'export-button',
  importInput: 'import-input',
  newDiagramButton: 'new-diagram-button',
  saveButton: 'save-button',
  saveDialogInput: 'save-dialog-input',
  searchInput: 'search-input',
  themeToggleButton: 'theme-toggle-button'
} as const;

export const C = {
  aiLiveEditor: 'ai_live_editor',
  editorChooserDismissedKey: 'mermaid-editor-chooser-dismissed',
  utmSource: 'mermaid_live_editor'
} as const;

export const MERMAID_THEMES = [
  'default',
  'neutral',
  'forest',
  'dark',
  'neo',
  'neo-dark',
  'redux',
  'redux-dark',
  'redux-color',
  'redux-dark-color'
] as const;

export const MERMAID_LOOKS = ['classic', 'handDrawn', 'neo'] as const;

export const defaultState: State = {
  code: `flowchart TD
    A[Christmas] -->|Get money| B(Go shopping)
    B --> C{Let me think}
    C -->|One| D[Laptop]
    C -->|Two| E[iPhone]
    C -->|Three| F[fa:fa-car Car]
  `,
  grid: true,
  // Empty on purpose: mermaid picks its own theme, look and layout per diagram type.
  mermaid: '{}',
  panZoom: true,
  rough: false,
  updateDiagram: true
};
