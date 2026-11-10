/* eslint-disable @typescript-eslint/no-non-null-assertion -- boundingBox is null-checked right before use */
import { expect, test } from './test';

const viewportTransform = async (page: import('@playwright/test').Page): Promise<string | null> =>
  page.locator('.svg-pan-zoom_viewport').getAttribute('transform');

test.describe('Canvas interaction', () => {
  test('space+drag pans the canvas', async ({ editPage }) => {
    const { page } = editPage;
    await editPage.checkTextInView('Christmas');
    const view = await editPage.view.boundingBox();
    expect(view).not.toBeNull();
    const before = await viewportTransform(page);
    await page.keyboard.down('Space');
    await page.mouse.move(view!.x + 100, view!.y + 100);
    await page.mouse.down();
    await page.mouse.move(view!.x + 200, view!.y + 180, { steps: 5 });
    await page.mouse.up();
    await page.keyboard.up('Space');
    const after = await viewportTransform(page);
    expect(after).not.toBeNull();
    expect(after).not.toBe(before);
  });

  test('plain empty-canvas drag does not pan', async ({ editPage }) => {
    const { page } = editPage;
    await editPage.checkTextInView('Christmas');
    const view = await editPage.view.boundingBox();
    expect(view).not.toBeNull();
    const before = await viewportTransform(page);
    await page.mouse.move(view!.x + 100, view!.y + 100);
    await page.mouse.down();
    await page.mouse.move(view!.x + 200, view!.y + 180, { steps: 5 });
    await page.mouse.up();
    const after = await viewportTransform(page);
    expect(after).toBe(before);
  });

  test('marquee selects multiple nodes and click clears', async ({ editPage }) => {
    const { page } = editPage;
    await editPage.clearEditor();
    await editPage.typeInEditor('flowchart LR\n A[Alpha] --> B[Beta]');
    await editPage.checkTextInView('Beta');
    const view = await editPage.view.boundingBox();
    expect(view).not.toBeNull();
    // Drag from the far-left edge across both nodes.
    await page.mouse.move(view!.x + 5, view!.y + view!.height / 2);
    await page.mouse.down();
    await page.mouse.move(view!.x + view!.width - 5, view!.y + view!.height / 2, { steps: 5 });
    await page.mouse.up();
    await expect(page.locator('.visual-selected')).toHaveCount(2);

    // A plain click on empty space clears the selection.
    await page.mouse.click(view!.x + 5, view!.y + 5);
    await expect(page.locator('.visual-selected')).toHaveCount(0);
  });
});
