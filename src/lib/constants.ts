import type { State } from './types';

export const TID = {
  aiEditBar: 'ai-edit-bar',
  aiEditInput: 'ai-edit-input',
  aiEditSend: 'ai-edit-send',
  aiHelpText: 'ai-help-text',
  aiRepairButton: 'ai-repair-button',
  authConfirm: 'auth-confirm',
  authEmail: 'auth-email',
  authError: 'auth-error',
  authPassword: 'auth-password',
  authSubmit: 'auth-submit',
  authSwitchLink: 'auth-switch-link',
  copyMarkdown: 'copy-markdown',
  dashboardPage: 'dashboard-page',
  diagramCard: 'diagram-card',
  diagramDocumentationButton: 'diagram-documentation-button',
  diagramsLink: 'diagrams-link',
  downloadButton: 'download-button',
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
  marquee: 'marquee-selection',
  navbarDownloadPNG: 'navbar-download-PNG',
  navbarDownloadSVG: 'navbar-download-SVG',
  navbarLogin: 'navbar-login',
  navbarLogout: 'navbar-logout',
  navbarUserMenu: 'navbar-user-menu',
  newDiagramButton: 'new-diagram-button',
  nodeLabelInput: 'node-label-input',
  saveButton: 'save-button',
  saveDialogFolderSelect: 'save-dialog-folder-select',
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
