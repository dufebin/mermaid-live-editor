// Ephemeral hand-tool (pan) mode for the canvas: toggled from the
// PanZoomToolbar button or the H key. While active, dragging anywhere pans
// the canvas — equivalent to holding Space. Never persisted.
let active = $state(false);

export const handTool = {
  get active(): boolean {
    return active;
  }
};

export const toggleHandTool = (): void => {
  active = !active;
};

export const setHandTool = (value: boolean): void => {
  active = value;
};
