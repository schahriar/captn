import type { SimulationNodeDatum } from 'd3';

export type NodeType = 'local' | 'vendor' | 'stdlib';

export interface GraphNode {
  id: string;
  label: string;
  type: NodeType | "unknown";
  behavior?: string;
  edgeCases?: string;
}

// D3 simulation mutates nodes in place, adding physics properties
export type SimNode = GraphNode & SimulationNodeDatum;

// Links start with string source/target; D3 resolves them to SimNode refs
export interface GraphLink {
  source: string | SimNode;
  target: string | SimNode;
  behavior?: string;
  edgeCases?: string;
}

export interface GraphData {
  nodes: GraphNode[];
  links: GraphLink[];
}

export interface HoveredNode extends SimNode {
  deg: number;
  pathToRoot: string[] | null;
}
