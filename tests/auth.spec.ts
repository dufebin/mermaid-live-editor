import { expect, test } from '@playwright/test';
import { TID } from '$/constants';

const register = async (page: import('@playwright/test').Page, email: string) => {
  await page.goto('/register');
  await page.getByTestId(TID.authEmail).fill(email);
  await page.getByTestId(TID.authPassword).fill('password123');
  await page.getByTestId(TID.authConfirm).fill('password123');
  await page.getByTestId(TID.authSubmit).click();
};

test.describe('Auth and per-user data', () => {
  test('register, save, isolate, redirect, public view', async ({ page }) => {
    await page.goto('/edit');
    await expect(page).toHaveURL(/\/login\?redirect=/);

    await register(page, 'a@e2e.test');
    await expect(page.getByTestId(TID.navbarUserMenu)).toBeVisible();
    await page.getByTestId(TID.saveButton).click();
    await page.getByTestId(TID.saveDialogInput).fill('A-diagram');
    await page.getByTestId(TID.saveDialogInput).press('Enter');
    await expect(page.getByText('Saved')).toBeVisible();

    await page.getByTestId(TID.navbarUserMenu).click();
    await page.getByTestId(TID.navbarLogout).click();
    await expect(page).toHaveURL(/\/login/);

    await register(page, 'b@e2e.test');
    // Wait for the register request to complete before navigating: goto would
    // cancel the in-flight POST and leave the session anonymous.
    await expect(page.getByTestId(TID.navbarUserMenu)).toBeVisible();
    await page.goto('/dashboard');
    await expect(page.getByTestId(TID.diagramCard)).toHaveCount(0);

    await page.getByTestId(TID.navbarUserMenu).click();
    await page.getByTestId(TID.navbarLogout).click();
    await page.goto(
      '/view#pako:eNpljkEKwzAMBL8SdM4Lcm7pA3rtRZGFbYqsIGRKCfl7DU5KIDrtrmZhVyANDBPE7A_DJb3K0I5UJHvXs2GhNBT-dLUTiemt1S_5qflnBHO5voUt8rkPI7SswaENWrdm6xLQ-R6yq8HkVnkErK7Pb6HDd-aWMRpKD7cfnfhJ6Q'
    );
    // gitGraph's rendered SVG shows branch labels, not the "gitGraph" keyword.
    await expect(page.locator('#view')).toContainText('newbranch');
  });

  test('login rejects wrong password with server message', async ({ page }) => {
    await register(page, 'c@e2e.test');
    await page.getByTestId(TID.navbarUserMenu).click();
    await page.getByTestId(TID.navbarLogout).click();
    await page.goto('/login');
    await page.getByTestId(TID.authEmail).fill('c@e2e.test');
    await page.getByTestId(TID.authPassword).fill('wrong-password');
    await page.getByTestId(TID.authSubmit).click();
    await expect(page.getByTestId(TID.authError)).toContainText('invalid email or password');
  });
});
