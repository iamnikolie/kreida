// Package templates holds the starter boards by type: `kreida new <name> --template <type>`. Each one shows the
// format of its kind of scheme, so an agent can start from a working board and rewrite its cards.
package templates

import "github.com/iamnikolie/kreida/internal/board"

// Template is a starter board.
type Template struct {
	Name  string
	About string
	yaml  func(title string) string
}

// YAML renders the template with a title.
func (t Template) YAML(title string) string { return t.yaml(title) }

// q writes a title as a YAML scalar.
func q(s string) string { return board.FormatScalar(s, false) }

// All lists the templates in the order `kreida templates` prints them.
func All() []Template {
	return []Template{
		{"flow", "process left to right: inputs -> processing -> results", func(t string) string {
			return "title: " + q(t) + `
sub: ""
direction: LR
legend: { src: Входы, core: Обработка, out: Результат }
nodes:
  a: { title: Начало, text: откуда приходит, kind: src }
  b: { title: Обработка, kind: core }
  c: { title: Результат, kind: out }
edges:
  - a -> b -> c
`
		}},
		{"lanes", "swimlane — who does what, in which order (client / assistant / human)", func(t string) string {
			return "title: " + q(t) + `
sub: кто что делает и в каком порядке
layout: lanes
legend: { src: Приходит, core: Делает ассистент, out: Решает человек }
groups:
  client: Клиент
  bot: Ассистент
  human: Редактор
nodes:
  c1: { title: Пишет запрос, text: Telegram или почта, kind: src, group: client, icon: message-circle }
  b1: { title: Разбирает запрос, kind: core, group: bot, icon: bot }
  b2: { title: Готовит ответ, text: черновик + подборка, kind: core, group: bot }
  h1: { title: Проверяет и жмёт «ОК», kind: out, group: human, icon: check }
  c2: { title: Получает ответ, kind: src, group: client }
edges:
  - c1 -> b1 -> b2 -> h1 -> c2
`
		}},
		{"sequence", "who sends what to whom, message by message (lanes + columns)", func(t string) string {
			return "title: " + q(t) + `
sub: сообщения по порядку
layout: lanes
groups:
  client: Клиент
  bot: Ассистент
  human: Редактор
nodes:
  m1: { title: Запрос, text: → Ассистент, group: client, step: 0 }
  m2: { title: Черновик ответа, text: → Редактор, group: bot, step: 1 }
  m3: { title: «ОК», text: → Ассистент, group: human, step: 2 }
  m4: { title: Ответ клиенту, text: → Клиент, group: bot, step: 3, kind: out }
edges:
  - m1 -> m2 -> m3 -> m4
`
		}},
		{"mindmap", "one topic branching into ideas (radial, a colour per branch)", func(t string) string {
			return "title: " + q(t) + "\nlayout: radial\nnodes:\n  root: { title: " + board.FormatScalar(t, true) + `, kind: who }
  a: Первая ветка
  a1: Идея
  a2: Идея
  b: Вторая ветка
  b1: Идея
  c: Третья ветка
  c1: Идея
  c2: Идея
edges:
  - root -> a
  - root -> b
  - root -> c
  - a -> a1
  - a -> a2
  - b -> b1
  - c -> c1
  - c -> c2
`
		}},
		{"matrix", "2×2 — what first: effect × effort, quick wins tinted", func(t string) string {
			return "title: " + q(t) + `
sub: что делать первым
layout: matrix
x: { label: Усилия, low: мало, high: много }
y: { label: Эффект, low: слабый, high: сильный }
hot: low-high
quadrants: { low-high: Быстрые победы, high-high: Большие ставки, low-low: Мелочи, high-low: Не сейчас }
nodes:
  a: { title: Шаблоны ответов, x: low, y: high }
  b: { title: Своя CRM, x: high, y: high }
  c: { title: Новый логотип, x: low, y: low }
  d: { title: Переезд на другой сервер, x: high, y: low }
`
		}},
		{"compare", "options × criteria as a table card, the decision as a note", func(t string) string {
			return "title: " + q(t) + `
direction: LR
nodes:
  options:
    title: Варианты
    cols: [Вариант, Цена, Скорость запуска, Риск]
    rows:
      - [n8n, { text: $20/мес, score: 2 }, { text: неделя, score: 1 }, { text: средний, score: 0 }]
      - [Zapier, { text: $70/мес, score: -1 }, { text: два дня, score: 2 }, { text: низкий, score: 1 }]
      - [Свой код, { text: $0, score: 2 }, { text: месяц, score: -1 }, { text: высокий, score: -1 }]
  pick: { title: Решение, kind: note, text: "**n8n**: дёшево и хватит на год.\n- Zapier, если нужно к пятнице\n- свой код потом" }
edges:
  - options -> pick
questions:
  - Какой бюджет в месяц?
  - { text: Когда нужен запуск, on: pick }
`
		}},
		{"schema", "data schema / ERD — tables with fields, FK rows linked", func(t string) string {
			return "title: " + q(t) + `
sub: стрелка = «один ко многим»
direction: LR
legend: { src: Уже есть, core: Новое }
nodes:
  clients: { title: clients, kind: src, icon: building-2, fields: [id, name, "status | клиент / лид"] }
  requests: { title: requests, kind: core, icon: inbox, fields: [id, "client_id -> clients", "city | город", "dates | даты"] }
  models: { title: models, kind: src, icon: user, fields: [id, name, "status | signed / not signed"] }
  picks: { title: request_picks, kind: core, fields: [id, "request_id -> requests", "model_id -> models", "why | почему подходит"] }
`
		}},
		{"before-after", "now vs with the assistant, revealed as two presentation steps", func(t string) string {
			return "title: " + q(t) + `
sub: → чтобы показать шаги
direction: LR
legend: { warn: Боль сейчас, core: Делает ассистент, out: Результат }
groups:
  now: Сейчас
  next: С ассистентом
nodes:
  in: { title: Запрос клиента, kind: who }
  n1: { title: Читает вручную, text: вечером, kind: warn, group: now }
  n2: { title: Ищет по таблицам, text: 30 минут, kind: warn, group: now }
  n3: { title: Отвечает, group: now }
  a1: { title: Ассистент разбирает, text: сразу, kind: core, group: next, icon: bot }
  a2: { title: Готовит подборку, text: 2 минуты, kind: core, group: next }
  a3: { title: Человек жмёт «ОК», kind: out, group: next }
edges:
  - in -> n1 -> n2 -> n3
  - in -> a1 -> a2 -> a3
steps:
  - { title: Сейчас, show: [now] }
  - { title: С ассистентом, show: [next], hide: [now] }
`
		}},
	}
}

// Get finds a template by name.
func Get(name string) (Template, bool) {
	for _, t := range All() {
		if t.Name == name {
			return t, true
		}
	}

	return Template{}, false
}

// Default is the starter for a board made without --template (and by the page's "new board" button).
func Default(title string) string {
	t, _ := Get("flow")

	return t.YAML(title)
}
