import { useCallback } from 'react';
import { getNodesBounds, getViewportForBounds, useReactFlow } from '@xyflow/react';
import { toPng } from 'html-to-image';

/** Saves the whole board (not just what is on screen) as `<fileName>.png`, in the current theme. */
export function useExportPng() {
  const flow = useReactFlow();
  return useCallback(async (fileName: string) => {
    const bb = getNodesBounds(flow.getNodes()), pad = 80, w = bb.width + pad * 2, h = bb.height + pad * 2;
    const vp = getViewportForBounds(bb, w, h, 0.2, 2, 0);
    const el = document.querySelector('.react-flow__viewport') as HTMLElement;
    const bg = getComputedStyle(document.documentElement).getPropertyValue('--bg').trim();
    // A step on screen fogs the rest of the board; the file has all of it.
    document.documentElement.classList.add('exporting');
    const url = await toPng(el, { backgroundColor: bg, width: w, height: h, pixelRatio: 2,
      style: { width: `${w}px`, height: `${h}px`, transform: `translate(${vp.x}px, ${vp.y}px) scale(${vp.zoom})` } })
      .finally(() => document.documentElement.classList.remove('exporting'));
    const a = document.createElement('a'); a.download = `${fileName}.png`; a.href = url; a.click();
  }, [flow]);
}
