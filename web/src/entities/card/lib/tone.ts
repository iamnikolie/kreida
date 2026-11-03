/** Colour of a live value's dot: values like ok/up/true read green, fail/down/error red, the rest neutral. */
export const tone = (v: string) => (/^(ok|up|green|pass|healthy|running|active|online|true|✓)/i.test(v) ? 'good'
  : /(fail|down|error|red|critical|offline|stopped|false|✗)/i.test(v) ? 'bad' : '');
