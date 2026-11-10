<script lang="ts">
  import { TID } from '$/constants';
  let { rect }: { rect: { height: number; left: number; top: number; width: number } } = $props();
</script>

<!-- pointer-events-none so the overlay never steals mouse events -->
<div
  class="pointer-events-none absolute z-10 rounded-sm border-2 border-accent bg-accent/10"
  data-testid={TID.marquee}
  style="height: {rect.height}px; left: {rect.left}px; top: {rect.top}px; width: {rect.width}px">
</div>
