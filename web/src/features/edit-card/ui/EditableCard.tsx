import { memo } from 'react';
import type { NodeProps } from '@xyflow/react';
import { Card, type CardData } from '@entities/card';
import { useCardEdit } from '../model/useCardEdit';

/** The card node of the canvas: an entity card made editable. */
export const EditableCard = memo(function EditableCard(p: NodeProps & { data: CardData }) {
  const edit = useCardEdit(p.id, p.data);
  return <Card {...p} edit={edit} />;
});
