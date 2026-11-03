import type { Diagram } from './api';

let current: Diagram | null = $state.raw(null);

export const currentDiagram = {
  get diagram(): Diagram | null {
    return current;
  }
};

export const setCurrentDiagram = (diagram: Diagram): void => {
  current = diagram;
};

export const clearCurrentDiagram = (): void => {
  current = null;
};
