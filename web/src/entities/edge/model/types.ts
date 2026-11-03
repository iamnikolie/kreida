export type XY = { x: number; y: number };

/** What the layout hands an edge: the route ELK computed and where it expects both ends to sit. */
export type RoutedData = { points?: XY[]; label?: XY; src?: XY; dst?: XY; sketch?: boolean; dashed?: boolean };
