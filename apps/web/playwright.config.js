import { defineConfig } from '@playwright/test';
export default defineConfig({ testDir: './tests', workers: 1, use: { baseURL: process.env.BNB_WEB_URL || 'http://127.0.0.1:5173', headless: true }, reporter: 'list' });
