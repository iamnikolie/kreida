---
name: kreida
description: Use when a quick visual scheme helps — a flow, process, architecture, plan, swimlanes, before/after, or a diagram to show a client on a call. You (the agent) write the board as YAML; it renders live on a React Flow canvas in the browser and the open page updates within a second of every change.
---

# kreida — live boards from the shell

You draw: read the source (chat, brief, call transcript, code), write the board YAML yourself, show it, look at it.
kreida has no AI of its own — the agent in the loop is the drawer.

```sh
kreida set workflow <<'YAML'             # create or replace a board (validated; broken YAML is rejected)
title: …
YAML
kreida open workflow                     # show it to the user in a browser; later changes appear live
kreida png workflow -o /tmp/s.png        # look at your result (Read the PNG) before saying it is done
                                      # --step N renders presentation step N
kreida cat workflow                      # current YAML — the human may have edited text or dragged cards
kreida undo workflow                     # step back (every saved version is kept: kreida history)
kreida export workflow -o workflow.html    # one read-only HTML file to send to a client or host
kreida share workflow                    # read-only live link for a client: /s/<token> (this machine)
kreida inbox workflow                    # notes the user left on the board for you (--json)
kreida resolve workflow n1           # close notes once you acted on them
kreida import flow < diagram.mmd       # Mermaid flowchart/graph or sequenceDiagram -> board (--force replaces)
kreida export workflow --mermaid         # board -> Mermaid flowchart (for docs, GitLab, Notion)
kreida new call --template before-after "Сейчас и с ассистентом"   # typed starters: kreida templates
kreida ls · kreida new <name> [title] · kreida path <name> · kreida rm <name> · kreida reset <name>
```

Loop on a call: the human says what to change → edit the YAML (`kreida set` with the full board, or edit
`kreida path <name>` in place) → the open page re-lays out and the changed cards glow.

Mermaid: flowchart import keeps subgraphs (→ groups, `direction` too), `:::src`-style classes named like kinds
(→ kind), `[(db)]` (→ database icon), `([pill])`, edge labels, dashed `-.->`, thick `==>`, invisible `~~~`.
A sequenceDiagram becomes a swimlane board: participants → lanes, messages → cards in order (replies `-->>` dashed).
`%% title:`, `%% sub:`, `%% legend: src Входы | core …` comments carry the board header both ways.
After import, read the YAML and tidy it (kinds, 2–5 word titles) — Mermaid has no colour roles.

Client link: `kreida share <name>` prints a read-only URL that follows every change live (no editing, no
other boards). It works on this machine only. `--lan` restarts the server so the link opens from other machines
on the network — those reach share links only, never the editor or other boards. Do this only when the user asks for it.
`kreida unshare <name>` revokes the link; `kreida up` goes back to local-only.

Source material can include transcripts, briefs, and existing documentation. Use material appropriate for the audience.

UI (for the human): changed cards glow after every update; ⌘Z / undo button steps back; drag cards
(positions persist in `<name>.pos.json`, the grid button resets); double-click a card to edit its text
(written back into the YAML); `p` presentation — ←/→ walk the board's `steps:` (default: top-level groups one by one), Home = everything; `f` fit, `t` theme,
`m` minimap; `s` sketch style (hand-drawn outlines and font, saved in the board as `style: sketch`); PNG button.
Positions persist until the card's place in the board changes (new, other group, links or group members changed).
View-only (`l`, the lock with «Просмотр»/«Правка» in the toolbar): on by default, remembered for every board in that
browser. While locked nothing on the canvas changes the board — no drag, no text edit, no undo, no relayout; pan, zoom,
glossary popovers, links, folding and notes still work. The user unlocks to drag or edit; you edit the YAML either way.
Right-click a card (or the empty board, or the speech-bubble button) → a note for the agent (see inbox below).
Click a group's header to fold it into one tile (title, card count, first titles; click the tile to unfold); the
toolbar folds every group to level 1/2/3 or unfolds all. Folding is a view state kept per board in the browser,
never a board edit; it works on share links too.

<!-- format -->
## Board format (YAML)

```yaml
title: Как команда обрабатывает запросы     # big header
sub: Ответ отправляется после проверки          # subtitle
direction: LR            # LR | TB | RL | BT — default for the board
legend: { src: Что приходит, core: Что делает ассистент, out: Результат для команды }
questions: [ Кто отвечает ночью? ]            # open questions panel (see below)
terms: { CRM: база клиентов и сделок }        # glossary: hover a mention

groups:                  # optional containers; id: label  or  id: {label, direction, parent, kind}
  s1: { label: Новая запись, direction: LR }
  s2: Запрос клиента

nodes:                   # map id -> "Title | secondary text"  or  {title, text, kind, group, icon, shape, width}
  a1: { title: Анкета в Telegram, text: "тема, описание", kind: src, group: s1, icon: send }
  b1: { title: "Заводит карточку | проверяет дубли", kind: core, group: s1 }
  reviewer: { title: Редактор, kind: who }
  idea: Просто карточка | без цвета

edges:                   # strings; chains allowed; ": label" at the end labels the last hop
  - a1 -> b1 -> c1
  - b1 -> reviewer: мало данных
  - x -.-> y             # dashed
  - x => y               # thick
  - s1 ~~~ s2            # invisible: layout order only
  - { from: a, to: b, label: да, animated: true }
```

Swimlanes (who does what, in which order): `layout: lanes`. Top-level groups become horizontal lanes,
columns come from edges (longest path) or an explicit `step: 0..n` on a node.

```yaml
layout: lanes
groups: { reviewer: Редактор, author: Автор }
nodes:
  k1: { title: Дать доступ, kind: src, group: reviewer }
  c1: { title: Изучить базу, kind: core, group: author }
  c0: { title: Мокап, kind: core, group: author, step: 0 }
edges: [ k1 -> c1 ]
```

Before/after: `direction: LR`, two groups with `direction: TB`, one edge `was => will: label` between the groups.

Templates (`kreida new <name> [title] --template <type>`, then rewrite the cards): `lanes` — who does what
(client / assistant / human); `sequence` — messages in order; `mindmap` — topic → branches → ideas, radial;
`matrix` — a 2×2 (effect × effort); `compare` — options × criteria table and the decision as a note; `schema` —
tables with fields; `before-after` — now vs with the assistant as two presentation steps; `flow` (default).
Pick by the question: "who does what" → lanes, "what happens in what order between people" → sequence,
"what do we store" → schema, "what changes for her" → before-after, "which option" → compare,
"what first" → matrix, "what could we do" → mindmap.

Live values: a card can show a number or status that refreshes while the board is open —
`live: { cmd: "…", every: 30, label: "…" }` (shell command, first line of stdout) or
`live: { url: "https://…", path: data.count, every: 60 }` (JSON field or text). Nothing runs until that exact
cmd/url is allowed for that board: `kreida live <name>` lists sources, `kreida live <name> --allow [node]`
approves them (an edited command is blocked again), `--revoke` drops them, `--run` prints current values.
Show the user the commands from `kreida live <name>` and get their approval before `--allow`. Blocked cards read
«не разрешено». Values like ok/up/true get a green dot, fail/down/error a red one. Share links show values,
never the commands.

Notes from the human (inbox): the user right-clicks a card (or the empty board) in the canvas → «Заметка для агента» and
writes what to change. `kreida inbox [name]`
prints the open ones, one block each (board, id, card, author, time, then the text; `--json` for scripts, `--all` adds
the closed). Read it before you redraw a board the user has been commenting on, act on each note, then
`kreida resolve <name> <id>…`. Unresolved notes stay in the inbox. Notes live in
`<name>.notes.json` beside the board and are visible in the editor.

Steps (a story told on a call — "now" → "with the assistant"): `steps:` lists what each →
reveals. A step is `{ title, show: [ids], hide: [ids] }` or just `a, b` (show). Ids are nodes or groups (a group
covers its cards). Anything some step shows starts hidden; items no step mentions stay visible the whole time
(shared context — the client, the inbox). Check each step: `kreida png <name> --step 2`.

```yaml
groups: { now: Сейчас, assist: С ассистентом }
nodes: { ... }
steps:
  - { title: Сейчас — всё руками, show: [now] }
  - { title: С ассистентом, show: [assist], hide: [now] }
```

ERD (tables): give a node `fields:` and it renders as a table, one row per field. A field is
`name`, `name | note`, `fk_id -> table` (FK) or `fk_id -.-> table` (optional FK, dashed). Each FK draws its own
edge from the referenced table's `id` row to the FK row (one → many) — don't repeat it under `edges:`.
`id` is shown as the key. Plain nodes and `edges:` mix freely with tables.

```yaml
direction: LR
nodes:
  cities: { title: cities, kind: core, fields: [id, name, country] }
  models:
    title: models
    kind: src
    fields: [id, name, "status | signed / not signed", "city_id -> cities", "agency_id -.-> agencies"]
  agencies: { title: agencies, kind: src, fields: [id, name, "terms | example terms"] }
```

Table card (a comparison, a scored decision): give a node `cols:` (header texts) and `rows:` (lists of cells). A cell
is text or `{ text, score }`; scores tint their cells from the table's lowest (red) to its highest (green). The first
cell of a row names it. Short rows are padded; a row longer than `cols` is an error. Not with `fields:`.

```yaml
nodes:
  options:
    title: Где хранить клиентов
    cols: [Вариант, Цена, Запуск]
    rows:
      - [Baserow, { text: $0, score: 2 }, { text: день, score: 2 }]
      - [Своя CRM, { text: $0, score: 2 }, { text: месяц, score: -2 }]
```

Note card (prose a client reads first — the framing, "what we decided"): `kind: note` with a multi-line `text:` reads
as markdown: `## heading`, `- item`, `1. item`, `**bold**`, `*italic*`, `` `code` ``, `[link](https://…)`. Links work
only for http(s) and mailto; nothing in the text is ever HTML. A note with prose is 300 px wide; set `width:` to change.

```yaml
  decision: { title: Что решили, kind: note, text: "## Baserow\n**Пример выбора**.\n- Airtable, если упрёмся в лимиты" }
```

Matrix (2×2 — "what first", risk, Eisenhower, SWOT): `layout: matrix`, axes `x:` and `y:` as `{ label, low, high }`,
each card `x: low|high` and `y: low|high` (high y is on top, high x on the right). `hot: <x>-<y>` tints one corner
(`low-high` = quick wins on effect × effort); `quadrants: { low-high: Быстрые победы, … }` labels them. Cards
without x/y line up under the matrix. Edges are allowed and run straight.

```yaml
layout: matrix
x: { label: Усилия, low: мало, high: много }
y: { label: Эффект, low: слабый, high: сильный }
hot: low-high
nodes:
  a: { title: Шаблоны ответов, x: low, y: high }
```

Radial mindmap: `layout: radial`. The first card no edge points to sits in the middle; edges make the tree around it,
each child of the root starts a colour family its branch keeps (plain cards; a `kind` keeps its own colour). Edges
run straight, without arrows. On a phone it falls back to a tree flowing down.

Open questions (what you still need from the client — the useful output of a call): a board-level `questions:` list
of texts or `{ text, done: true, on: <node or group id> }`. They show in a panel under the legend (in the phone's ⋯
sheet), open first, done struck through; a card with an open question gets a «?» badge and clicking the question
shows the card. Mark one done by editing the YAML; don't fake questions with `warn` cards.

Glossary: a board-level `terms:` map — `CRM: база клиентов` or `webhook: { text: "…", aka: [вебхук, вебхуки] }`.
Every mention in a card's title or text (whole word, any case, aliases too) gets a dotted underline and explains
itself on hover or tap. Put in the jargon a client may not know; skip the obvious.

Folding: a group with `collapsed: true` starts folded into one tile (the reader can unfold it); the tile carries the
note and «?» badges of the cards inside. Use it for a big board whose detail groups a client opens on demand.

Language: what the page draws on the canvas itself (the questions panel, a tile's card count, badge tooltips) speaks
the board's language — Ukrainian or Russian by its Cyrillic, English by its Latin. Say it outright with
`lang: ru|uk|en` at the top when the board mixes scripts.

Sketch style: `style: sketch` at the top of the board — hand-drawn outlines, wobbly edges, Caveat font
(Cyrillic included). Good for early ideas on a call ("this is a draft, argue with it"); keep the clean style
for the final scheme a client keeps.

Kinds (colour roles): `src` blue (inputs), `core` amber (processing), `out` green (results),
`warn` red (risk/problem), `info` violet, `who` dark pill (a person/role), `note` yellow sticky,
`p2` dashed grey (later / out of scope), `plain` (default, white). Groups take `kind: p2` for a dashed zone.

Icons: any lucide name in kebab-case (`mail`, `send`, `database`, `user`, `bot`, `calendar`, `file-text`,
`search`, `bell`, `message-circle`, `shield-check`, `sparkles`, `clock`, `users`, `inbox`).
Brand icons come by themselves: a card whose title names a known tool (Telegram/телеграм, Gmail, GMass, Baserow,
Gemini, Claude, Google Sheets/гугл таблицы, Google Drive, WhatsApp, Instagram, Notion, Airtable, n8n, Zapier,
Stripe, GitHub/GitLab, MySQL, Postgres, Redis, Qdrant, Slack, OpenAI/ChatGPT/GPT, LinkedIn, Excel/эксель, Fibery…)
shows its logo. Force one with `icon: telegram` (brand keys: telegram gmail baserow gemini claude sheets drive
calendar docs meet notion whatsapp instagram viber slack openai linkedin excel fibery …), turn it off with `icon: none`.

Picture on a card: `image: <local path>` on a plain card or a note (not a table, not a pill). Shows on the canvas, in
`kreida png`, `kreida export` and on share links, above the card's text. Default card width is 300 px; set `width:` for more.

```yaml
nodes:
  shot: { title: Экран заказа, kind: out, image: img/order.png, width: 420 }   # next to the board file
  logo: { title: Клиент, image: ~/work/acme/logo.webp }                        # or under home, or absolute
```

A relative path is relative to the directory of the board file (`kreida path <name>` shows it), never to where you run
the command; `kreida set` reads stdin and does not know your directory. To keep a board portable, keep its images beside
it and use relative paths. URLs are refused.

Limits, checked when the file is read: PNG, JPEG, GIF or WebP judged by content, not by the file's name (SVG is refused:
it can run scripts); a regular file (a link is followed and its target checked the same way, so a link to a text file is
refused); at most 5 MiB and 40 megapixels per file, 25 MiB per board.

A missing or refused file never fails a board. `kreida set`, `png`, `export` and `publish` name the card and the path on
stderr and exit 0. Your own editor and `kreida png` draw a slot saying why; an export, a share link and a published board
show the card without a picture and without the reason. Read stderr before you send a client an export.

What leaves the machine: the path never does, in any JSON or file, only the bytes, as they are (a photo's EXIF stays in
them). An export or a publish carries the bytes of any image the board names, so check where a board's paths point before
you send it. On a published board only that board's images go up. A changed image file reaches the open page and, for a
published board, the cloud within seconds, by the same route as a changed board.

Inside `{ … }` quote any value that has a comma, colon or `|`: `text: "тема, описание"`.

Layout is automatic (ELK). When an edge crosses a group border, the whole board flows in the board's
`direction` and a group's own `direction` is ignored — draw before/after with `direction: LR` and edges from
a shared card into each group. Node order in the file is kept. Groups without edges inside lay their nodes in a line
in the group's direction; top-level groups without edges between them stack in the board's direction —
so "rows of scenarios" = `direction: TB` on the board + `direction: LR` on each group.
Edges may point to a group id. Unknown ids in edges become plain nodes.

Style rules that keep boards readable: 2–5 word titles, secondary text one short line,
≤ 20 nodes per board (split into several boards otherwise), one idea per group, legend whenever kinds are used.
