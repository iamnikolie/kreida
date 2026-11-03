// The JSON contract with the backend: these are exactly the shapes it sends. Do not reshape them here.
export type Dir = 'LR' | 'RL' | 'TB' | 'BT';

export interface BField { name: string; ref: string | null; dashed: boolean; note: string }
export interface BLive { source: string; every: number; path: string | null; label: string }
export interface LiveVal { value?: string; error?: string; blocked?: boolean; at?: string; source?: string }
export interface BNode {
  id: string; title: string; text: string; kind: string;
  group: string | null; icon: string | null; shape: string | null; width: number | null; step: number | null;
  fields: BField[] | null;
  live: BLive | null;
  /** A table card: header texts and rows of cells (a score tints a cell). */
  cols?: string[]; rows?: BCell[][];
  /** Its quadrant on a matrix board. */
  x?: Corner; y?: Corner;
}
export interface BCell { text: string; score?: number }
export type Corner = 'low' | 'high';
export interface BAxis { label: string; low: string; high: string }
/** `layout: matrix`: the axes, the hot corner and the quadrant labels; corners are "<x>-<y>" ("low-high"). */
export interface BMatrix { x: BAxis; y: BAxis; hot?: string; quadrants: Record<string, string> }
export interface BQuestion { text: string; done?: boolean; on?: string }
export interface BTerm { term: string; text: string; aka: string[] }
export interface BGroup { id: string; label: string; kind: string | null; direction: Dir | null; parent: string | null; collapsed?: boolean }
export interface BEdge {
  id: string; from: string; to: string; label: string;
  dashed?: boolean; hidden?: boolean; thick?: boolean; animated?: boolean;
  fromPort?: string; toPort?: string; // table field rows (FK edges)
}
export interface BStep { title: string; show: string[]; hide: string[] }
export interface Board {
  title: string; sub: string; direction: Dir; layout: 'auto' | 'lanes' | 'matrix' | 'radial'; style: 'clean' | 'sketch';
  legend: { kind: string; label: string }[];
  groups: BGroup[]; nodes: BNode[]; edges: BEdge[]; steps: BStep[];
  matrix?: BMatrix; questions?: BQuestion[]; terms?: BTerm[];
  /** The language of the board's words, when the file names it. */
  lang?: 'ru' | 'uk' | 'en';
}
export type Pos = Record<string, { x: number; y: number }>;
export interface BoardResp {
  name: string; board?: Board; pos?: Pos; error?: string;
  /** Share link of kreida cloud only: the owner's display name in the link's language. Absent or empty = no name line. */
  owner?: string;
  /** Share link of kreida cloud only: the link takes comments from its holder. Absent = no composer. */
  comments?: boolean;
}
/** A note for the agent: written on a card (or the board) in the editor, or a client's comment on a cloud link. Plain text. */
export interface Note { id: string; node: string; text: string; author: string; created: string; resolved?: string }
export interface BoardItem { name: string; title: string; mtime: number }
export type BoardStyle = Board['style'];
export type LiveValues = Record<string, LiveVal>;
