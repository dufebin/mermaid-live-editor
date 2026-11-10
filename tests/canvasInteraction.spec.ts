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
});
