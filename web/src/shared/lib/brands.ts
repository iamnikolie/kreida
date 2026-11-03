// Brand icons for known tools (simple-icons, CC0; bundled, nothing is fetched at render time).
// A card gets one when `icon:` names a brand key, or — with no `icon:` — when its title mentions the tool.
import {
  siAirtable, siAnthropic, siBaserow, siCalendly, siClaude, siCloudflare, siDiscord, siDocker, siDropbox, siFacebook,
  siFigma, siGithub, siGitlab, siGmail, siGooglecalendar, siGoogledocs, siGoogledrive, siGooglegemini, siGooglemeet,
  siGooglesheets, siHubspot, siInstagram, siJira, siMailchimp, siMake, siMiro, siMysql, siN8n, siNotion, siPaypal,
  siPerplexity, siPostgresql, siQdrant, siRedis, siShopify, siStripe, siSupabase, siTelegram, siTiktok, siTrello,
  siViber, siWhatsapp, siWise, siWordpress, siYoutube, siZapier, siZoom, type SimpleIcon,
} from 'simple-icons';
import { siFibery, siLinkedin, siMicrosoftexcel, siOpenai, siSlack } from './marks';

// key -> icon + names it goes by (lowercase; Latin and Cyrillic). Key = what `icon:` accepts.
const BRANDS: Record<string, [SimpleIcon, string[]]> = {
  telegram: [siTelegram, ['telegram', 'телеграм', 'телеграмм', 'тг']],
  gmail: [siGmail, ['gmail', 'gmass', 'джимейл']],
  baserow: [siBaserow, ['baserow', 'бейзроу']],
  gemini: [siGooglegemini, ['gemini', 'джемини']],
  claude: [siClaude, ['claude', 'клод']],
  anthropic: [siAnthropic, ['anthropic']],
  sheets: [siGooglesheets, ['google sheets', 'google таблиц', 'гугл таблиц', 'гугл-таблиц']],
  drive: [siGoogledrive, ['google drive', 'гугл диск', 'гугл-диск', 'google диск']],
  calendar: [siGooglecalendar, ['google calendar', 'гугл календар']],
  docs: [siGoogledocs, ['google docs', 'гугл док']],
  meet: [siGooglemeet, ['google meet']],
  notion: [siNotion, ['notion', 'ноушн']],
  github: [siGithub, ['github']],
  gitlab: [siGitlab, ['gitlab']],
  whatsapp: [siWhatsapp, ['whatsapp', 'ватсап', 'вотсап']],
  instagram: [siInstagram, ['instagram', 'инстаграм', 'инста']],
  viber: [siViber, ['viber', 'вайбер']],
  facebook: [siFacebook, ['facebook', 'фейсбук']],
  youtube: [siYoutube, ['youtube', 'ютуб']],
  tiktok: [siTiktok, ['tiktok', 'тикток']],
  discord: [siDiscord, ['discord', 'дискорд']],
  zoom: [siZoom, ['zoom', 'зум']],
  airtable: [siAirtable, ['airtable']],
  figma: [siFigma, ['figma', 'фигма']],
  stripe: [siStripe, ['stripe', 'страйп']],
  paypal: [siPaypal, ['paypal']],
  wise: [siWise, ['wise']],
  zapier: [siZapier, ['zapier']],
  n8n: [siN8n, ['n8n']],
  make: [siMake, ['make.com']],
  calendly: [siCalendly, ['calendly']],
  hubspot: [siHubspot, ['hubspot']],
  trello: [siTrello, ['trello', 'трелло']],
  jira: [siJira, ['jira', 'джира']],
  miro: [siMiro, ['miro', 'миро']],
  mailchimp: [siMailchimp, ['mailchimp']],
  shopify: [siShopify, ['shopify']],
  wordpress: [siWordpress, ['wordpress']],
  dropbox: [siDropbox, ['dropbox']],
  mysql: [siMysql, ['mysql']],
  postgres: [siPostgresql, ['postgres', 'postgresql']],
  redis: [siRedis, ['redis']],
  qdrant: [siQdrant, ['qdrant']],
  supabase: [siSupabase, ['supabase']],
  docker: [siDocker, ['docker']],
  cloudflare: [siCloudflare, ['cloudflare']],
  perplexity: [siPerplexity, ['perplexity']],
  slack: [siSlack, ['slack', 'слак']],
  openai: [siOpenai, ['openai', 'open ai', 'chatgpt', 'gpt', 'опенаи']],
  linkedin: [siLinkedin, ['linkedin', 'линкедин', 'линкедін']],
  excel: [siMicrosoftexcel, ['excel', 'эксель', 'экселе', 'экселя', 'ексель']],
  fibery: [siFibery, ['fibery', 'файбери']],
};

const ALIASES = Object.entries(BRANDS).flatMap(([k, [, names]]) => names.map(n => [n, k] as const))
  .sort((a, b) => b[0].length - a[0].length);
const esc = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
// Whole word, Latin or Cyrillic; longer Cyrillic aliases may take case endings (телеграм-е, таблиц-ы),
// short ones stay exact so «миро» never matches «мировой».
const MATCH = ALIASES.map(([n, k]) => [new RegExp(`(^|[^\\p{L}\\p{N}])${esc(n)}${/[а-яё]$/i.test(n) && n.length >= 6 ? '\\p{L}{0,3}' : ''}($|[^\\p{L}\\p{N}])`, 'iu'), k] as const);

export function brandOf(icon: string | null, title: string): SimpleIcon | null {
  if (icon) return BRANDS[icon.toLowerCase()]?.[0] ?? null; // explicit icon: a brand key, a lucide name, or none
  for (const [re, k] of MATCH) if (re.test(title)) return BRANDS[k][0];
  return null;
}

// Brand colour, unless it is too dark to read on a dark card (then the card's ink).
export function brandColor(i: SimpleIcon) {
  const n = parseInt(i.hex, 16), l = (0.299 * (n >> 16) + 0.587 * ((n >> 8) & 255) + 0.114 * (n & 255)) / 255;
  return l < 0.25 ? 'currentColor' : `#${i.hex}`;
}

// Whether a card draws an icon (layout reserves room for it).
export const hasIcon = (n: { icon: string | null; title: string }) => n.icon !== 'none' && (!!n.icon || !!brandOf(null, n.title));

export const BRAND_KEYS = Object.keys(BRANDS);
