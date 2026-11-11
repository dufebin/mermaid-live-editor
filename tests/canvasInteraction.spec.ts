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

  test('shift+click toggles selection membership', async ({ editPage }) => {
    const { page } = editPage;
    await editPage.clearEditor();
    await editPage.typeInEditor('flowchart LR\n A[Alpha] --> B[Beta]');
    await editPage.checkTextInView('Beta');
    const alpha = page.locator('#view .node').filter({ hasText: 'Alpha' });
    const beta = page.locator('#view .node').filter({ hasText: 'Beta' });
    await alpha.click();
    await expect(page.locator('.visual-selected')).toHaveCount(1);
    await beta.click({ modifiers: ['Shift'] });
    await expect(page.locator('.visual-selected')).toHaveCount(2);
    await beta.click({ modifiers: ['Shift'] });
    await expect(page.locator('.visual-selected')).toHaveCount(1);
  });

  test('group drag moves selected nodes together', async ({ editPage }) => {
    const { page } = editPage;
    await editPage.clearEditor();
    await editPage.typeInEditor('flowchart LR\n A[Alpha] --> B[Beta]');
    await editPage.checkTextInView('Beta');
    const view = await editPage.view.boundingBox();
    expect(view).not.toBeNull();
    // Marquee-select both nodes.
    await page.mouse.move(view!.x + 5, view!.y + view!.height / 2);
    await page.mouse.down();
    await page.mouse.move(view!.x + view!.width - 5, view!.y + view!.height / 2, { steps: 5 });
    await page.mouse.up();
    await expect(page.locator('.visual-selected')).toHaveCount(2);

    // Drag one of them; both must move by the same delta.
    const alpha = page.locator('#view .node').filter({ hasText: 'Alpha' });
    const beta = page.locator('#view .node').filter({ hasText: 'Beta' });
    const box = await alpha.boundingBox();
    expect(box).not.toBeNull();
    await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2);
    await page.mouse.down();
    await page.mouse.move(box!.x + box!.width / 2 + 60, box!.y + box!.height / 2 + 40, {
      steps: 5
    });
    await page.mouse.up();

    const translateOf = (locator: import('@playwright/test').Locator) =>
      locator.evaluate((element) => (element as SVGElement).style.translate);
    const t1 = await translateOf(alpha);
    const t2 = await translateOf(beta);
    expect(t1).not.toBe('');
    expect(t1).not.toBe('0px 0px');
    expect(t1).toBe(t2);
  });

  test('hand tool toggles pan mode on and off', async ({ editPage }) => {
    const { page } = editPage;
    await editPage.checkTextInView('Christmas');
    const view = await editPage.view.boundingBox();
    expect(view).not.toBeNull();

    // Activate: plain empty-canvas drag pans (no marquee), like space-pan.
    await page.getByTestId('hand-tool').click();
    const before = await viewportTransform(page);
    await page.mouse.move(view!.x + 100, view!.y + 100);
    await page.mouse.down();
    await page.mouse.move(view!.x + 200, view!.y + 180, { steps: 5 });
    await page.mouse.up();
    const afterPan = await viewportTransform(page);
    expect(afterPan).not.toBe(before);

    // Deactivate: the same drag no longer pans (back to marquee mode).
    await page.getByTestId('hand-tool').click();
    const before2 = await viewportTransform(page);
    await page.mouse.move(view!.x + 100, view!.y + 100);
    await page.mouse.down();
    await page.mouse.move(view!.x + 200, view!.y + 180, { steps: 5 });
    await page.mouse.up();
    expect(await viewportTransform(page)).toBe(before2);

    // H key toggles it too (and Escape turns it off).
    await page.keyboard.press('h');
    const before3 = await viewportTransform(page);
    await page.mouse.move(view!.x + 100, view!.y + 100);
    await page.mouse.down();
    await page.mouse.move(view!.x + 180, view!.y + 150, { steps: 5 });
    await page.mouse.up();
    expect(await viewportTransform(page)).not.toBe(before3);
    await page.keyboard.press('Escape');
    await expect(page.getByTestId('hand-tool')).toHaveAttribute('aria-pressed', 'false');
  });
});
