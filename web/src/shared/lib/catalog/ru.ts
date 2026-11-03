// The Russian catalog is the base: its keys are the app's keys. uk.ts and en.ts are typed after it, so the compiler
// names what is missing, and i18n.test.ts fails on a key that any language lacks. Keys are dotted, grouped by the
// widget they label; `{name}` is a value filled in by `t(key, { name })`. Board content (titles, texts, the legend's
// labels) is the author's and never goes through a catalog.

export const ru = {
  // the icon row at the top right (a key in brackets is the hotkey)
  'tool.bar': 'Инструменты',
  'tool.undo': 'Отменить (⌘Z)',
  'tool.fit': 'Вписать (f)',
  'tool.relayout': 'Авто-раскладка (сбросить ручные позиции)',
  'tool.minimap': 'Мини-карта (m)',
  'tool.theme': 'Тема (t)',
  'tool.sketch': 'Скетч (s)',
  'tool.png': 'Скачать PNG',
  'tool.present': 'Показ (p), шаги ← →',
  // the presentation bar of a desktop viewer
  'present.label': 'Шаги',
  'present.all': 'всё',
  // a kreida cloud link: whose board it is, and the way back to kreida
  'head.from': 'От: {name}',
  'credit.made': 'Сделано в kreida',
  // a phone's bottom bar and its sheet
  'bar.label': 'Управление схемой',
  'bar.all': 'Вся схема',
  'bar.none': 'всё',
  'bar.step': 'Шаг {n} из {total}',
  'bar.prev': 'Назад',
  'bar.next': 'Вперёд',
  'bar.more': 'Ещё',
  'sheet.legend': 'Легенда',
  'sheet.theme': 'Тема',
  'sheet.png': 'Скачать PNG',
  'sheet.close': 'Закрыть',
  // messages over the canvas
  'notice.empty': 'Досок нет. Попроси агента нарисовать схему:',
  'notice.expired': 'Ссылка не работает',
  'error.layout': 'Не удалось разложить схему: {detail}',
  'error.undo': 'Не удалось отменить',
  // a live card
  'live.blocked': 'не разрешено',
  'live.error': 'ошибка',
  // the editor
  'card.description': 'описание',
  'switch.label': 'Доска',
  'switch.new': '+ Новая доска…',
  'switch.name': 'имя доски, Enter',
  'switch.badName': 'имя: латиница, цифры, - _ .',
  // a card's corner badges
  'card.asks.one': '{n} открытый вопрос',
  'card.asks.few': '{n} открытых вопроса',
  'card.asks.many': '{n} открытых вопросов',
  'card.asks.other': '{n} открытого вопроса',
  'card.notes.one': '{n} заметка для агента',
  'card.notes.few': '{n} заметки для агента',
  'card.notes.many': '{n} заметок для агента',
  'card.notes.other': '{n} заметки для агента',
  // folding groups: a group's header and its tile, the toolbar's levels
  'fold.open': 'Развернуть группу',
  'fold.close': 'Свернуть группу',
  'fold.count.one': '{n} карточка',
  'fold.count.few': '{n} карточки',
  'fold.count.many': '{n} карточек',
  'fold.count.other': '{n} карточки',
  'fold.label': 'Группы',
  'fold.level': 'Свернуть до уровня {n}',
  'fold.all': 'Развернуть все группы',
  'sheet.fold': 'Свернуть группы',
  'sheet.unfold': 'Развернуть группы',
  // the board's open questions
  'questions.title': 'Открытые вопросы',
  'questions.count': '{open} из {total}',
  'questions.done': 'решено',
  'sheet.questions': 'Открытые вопросы',
  // notes for the agent (the editor) and a client's comments (a kreida cloud link)
  'note.card': 'Заметка для агента',
  'note.board': 'Заметка ко всей схеме',
  'note.comment': 'Комментарий',
  'note.commentBoard': 'Комментарий к схеме',
  'note.placeholder': 'Что поменять или проверить',
  'note.send': 'Отправить',
  'note.cancel': 'Отмена',
  'note.sent': 'Отправлено',
  'note.failed': 'Не удалось отправить',
  'note.you': 'вы',
  'note.done': 'учтено',
  // view-only: nothing on the canvas changes the board
  'lock.view': 'Просмотр',
  'lock.edit': 'Правка',
  'lock.title': 'Просмотр без правок (l): карточки не двигаются и не редактируются',
  // the PNG's file name when the board has none
  'export.name': 'схема',
};

export type Key = keyof typeof ru;
export type Catalog = Record<Key, string>;
