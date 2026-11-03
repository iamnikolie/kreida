import { useMemo, useState } from 'react';
import { Panel } from '@xyflow/react';
import { NoteComposer, useNotes, type NoteTarget } from '@features/card-note';
import { FitContext, useFitView } from '@features/fit-view';
import { useFolds } from '@features/fold-groups';
import { usePlay, usePresentation } from '@features/present';
import { useSketch } from '@features/sketch-toggle';
import { useTheme } from '@features/theme';
import { useUndo } from '@features/undo';
import { useViewLock } from '@features/view-lock';
import { foldedInto, ownerOf, rollUp, useBoardSource } from '@entities/board';
import {
  BoardActionsContext, BoardLangContext, GlossaryContext, boardLang, LockContext, compileGlossary, t, useEdges, useMode, usePhone, useToast, useToggle, type BoardActions,
} from '@shared/lib';
import { MadeWith } from '@shared/ui';
import { Canvas } from '@widgets/canvas';
import { Head } from '@widgets/head';
import { Legend } from '@widgets/legend';
import { Notice } from '@widgets/notice';
import { PhoneBar } from '@widgets/phone-bar';
import { Questions } from '@widgets/questions';
import { StepsBar } from '@widgets/steps-bar';
import { Toolbar } from '@widgets/toolbar';
import { useBoardHotkeys } from '../model/useBoardHotkeys';
import './board-page.css';

/**
 * One board on a full-screen canvas. Holds the state several widgets share and wires the keyboard.
 * On a phone (a share link held upright) the toolbar, the legend and the steps bar give way to one bottom bar.
 */
export function BoardPage() {
  const { readonly, mode, cloud } = useMode();
  const phone = usePhone();
  const { msg, toast } = useToast();
  const { list, name, setName, resp, reloadList } = useBoardSource();
  const board = resp?.board;
  const fileName = resp?.name ?? name ?? t('export.name');
  const owner = ownerOf(resp, mode);

  const presentation = usePresentation(board);
  usePlay(presentation); // ?play=1: the demo plays itself until the visitor takes over
  const theme = useTheme();
  const sketch = useSketch(name, board?.style);
  const minimap = useToggle();
  const undo = useUndo(name, toast);
  const lock = useViewLock();
  const folds = useFolds(name, board);
  const notes = useNotes(name, resp);
  const [note, setNote] = useState<NoteTarget | null>(null);
  const lang = useMemo(() => (board ? boardLang(board) : null), [board]);
  const glossary = useMemo(() => compileGlossary(board?.terms ?? []), [board?.terms]);
  const actions = useMemo<BoardActions>(() => ({
    fold: folds.depth ? folds.toggle : undefined,
    note: notes.can ? (node, at) => setNote({ node, label: board?.nodes.find(n => n.id === node)?.title ?? board?.groups.find(g => g.id === node)?.label, ...at }) : undefined,
  }), [folds.depth, folds.toggle, notes.can, board]);
  // badges: a folded tile carries the open questions and notes of the cards inside it
  const marks = useMemo(() => {
    const asks = new Map<string, number>();
    for (const q of board?.questions ?? []) if (q.on && !q.done) asks.set(q.on, (asks.get(q.on) ?? 0) + 1);
    const into = board && board.layout !== 'lanes' ? foldedInto(board, folds.folds) : (id: string) => id;
    return { asks: rollUp(asks, into), notes: rollUp(notes.open, into) };
  }, [board, folds.folds, notes.open]);
  const questions = board?.questions?.length ? <Questions items={board.questions} /> : null;
  const boardNote = notes.can ? (at: { x: number; y: number }) => setNote({ node: '', ...at }) : undefined;
  // what a fit needs to know: how tall the phone's title bar is, whether there are steps to walk, and what is laid over the
  // canvas (a client's desktop frames the board between the head and legend above and the steps bar below)
  const [headRef, head] = useEdges(), [sideRef, side] = useEdges(), [barRef, bar] = useEdges();
  const top = Math.max(head.bottom, side.bottom), foot = bar.top ? innerHeight - bar.top : 0;
  const fitOpts = useMemo(() => ({ owned: !!owner, stepped: presentation.stepped, top, foot }), [owner, presentation.stepped, top, foot]);
  const fit = useFitView(fitOpts);
  useBoardHotkeys({ presentation, theme, sketch, minimap, fit, undo, lock });

  return (
    <FitContext value={fitOpts}>
      <LockContext value={lock.locked}><BoardLangContext value={lang}><GlossaryContext value={glossary}><BoardActionsContext value={actions}>
      <div className={`app${sketch.on ? ' sketch' : ''}${phone ? ' phone' : ''}${mode === 'share' ? ' share' : ''}${cloud ? ' cloud' : ''}${presentation.on ? ' presenting' : ''}${lock.locked ? ' locked' : ''}`}>
        <Canvas resp={resp} name={name} theme={theme.theme} sketch={sketch.on} presentation={presentation} minimap={minimap.on}
          folds={folds.folds} marks={marks} onError={toast}>
          <Head ref={headRef} board={board} owner={owner} presenting={presentation.on}
            switcher={{ list, name, onSelect: setName, reload: reloadList, onError: toast }} />

          {phone ? (
            <PhoneBar presentation={presentation} theme={theme} fileName={fileName}
              legend={board?.legend.length ? <Legend items={board.legend} /> : null} questions={questions}
              fold={folds.depth ? { folded: folds.folds.size > 0, toggle: () => (folds.folds.size ? folds.unfoldAll() : folds.level(1)) } : null}
              onNote={boardNote && (() => boardNote({ x: 8, y: 80 }))} />
          ) : (
            <>
              <Panel ref={sideRef} position="top-right" className="side">
                <Toolbar board={name} fileName={fileName} presentation={presentation} theme={theme}
                  sketch={sketch} minimap={minimap} undo={undo} lock={lock} folds={folds} onNote={boardNote} />
                <Legend items={board?.legend ?? []} />
                {questions}
              </Panel>
              <StepsBar ref={barRef} presentation={presentation} />
              <MadeWith className="credit" />
            </>
          )}

          <Notice error={msg || resp?.error || ''} empty={!readonly && !list.length} />
        </Canvas>
        {note && <NoteComposer notes={notes} target={note} onClose={() => setNote(null)} onSent={toast} />}
      </div>
      </BoardActionsContext></GlossaryContext></BoardLangContext></LockContext>
    </FitContext>
  );
}
