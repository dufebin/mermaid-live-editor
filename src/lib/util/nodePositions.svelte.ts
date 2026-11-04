export interface NodeOffset {
  dx: number;
  dy: number;
}

let positions: Record<string, NodeOffset> = $state.raw({});
let zoom = 1;

export const nodePositions = {
  get value(): Record<string, NodeOffset> {
    return positions;
  },
  get zoom(): number {
    return zoom;
  }
};

export const setNodePosition = (name: string, dx: number, dy: number): void => {
  positions = { ...positions, [name]: { dx, dy } };
};

export const setZoom = (value: number): void => {
  zoom = value;
};

export const resetNodePositions = (next: Record<string, NodeOffset> = {}): void => {
  positions = next;
};
