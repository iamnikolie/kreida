import { expect, it } from 'vitest';
import { brandOf, hasIcon } from './brands';

const t = (title: string) => brandOf(null, title)?.title ?? null;

it('auto brand icon from the title, Latin and Cyrillic, whole words', () => {
  expect(t('Анкета в Telegram')).toBe('Telegram');
  expect(t('Пишет в телеграме')).toBe('Telegram');
  expect(t('Ищет в Baserow')).toBe('Baserow');
  expect(t('Сводка из гугл таблицы')).toBe('Google Sheets');
  expect(t('Gemini разбирает фото')).toBe('Google Gemini');
  expect(t('Рассылки GMass')).toBe('Gmail');
  expect(t('Тред в Slack')).toBe('Slack');
  expect(t('ChatGPT пишет черновик')).toBe('OpenAI');
  expect(t('GPT-4o')).toBe('OpenAI');
  expect(t('Профиль в LinkedIn')).toBe('LinkedIn');
  expect(t('Выгрузка в экселе')).toBe('Microsoft Excel');
  expect(t('Тикет в Fibery')).toBe('Fibery');
  expect(t('Excellent')).toBeNull();
  expect(t('Мировой рынок')).toBeNull();
  expect(t('Instagrammer')).toBeNull();
  expect(t('Просто карточка')).toBeNull();
});

it('explicit icon: brand key wins, lucide names and none give no brand', () => {
  expect(brandOf('claude', 'x')?.title).toBe('Claude');
  for (const k of ['slack', 'openai', 'linkedin', 'excel', 'fibery']) expect(brandOf(k, 'x')).not.toBeNull();
  expect(brandOf('mail', 'Gmail')).toBeNull();
  expect(hasIcon({ icon: 'none', title: 'Telegram' })).toBe(false);
  expect(hasIcon({ icon: null, title: 'Telegram' })).toBe(true);
});
