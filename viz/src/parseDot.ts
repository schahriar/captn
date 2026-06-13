import type { NodeType, GraphNode, GraphData } from './types.js';

const goTypeToNodeType: Record<string, NodeType> = {
  local:      'local',
  dependency: 'vendor',
  stdlib:     'stdlib',
};

function makeNode(id: string, typeMap: Map<string, NodeType>): GraphNode {
  return {
    id,
    label: id.split('/').pop() ?? id,
    type:  typeMap.get(id) ?? 'unknown',
  };
}

/**
 * Parses a DOT string (strict graph or digraph) into { nodes, links }.
 * Reads import_type vertex attributes for node classification.
 * Deduplicates edges that appear in both directions (common in undirected output).
 */
export function parseDOT(src: string): GraphData {
  const nodes   = new Map<string, GraphNode>();
  const edgeSet = new Set<string>();
  const links: GraphData['links'] = [];

  // First pass: collect import_type from vertex declarations.
  // Vertex lines look like:  "path" [ import_type="local",  weight=0 ]
  // Edge lines look like:    "path" -- "other" [ weight=0 ]
  // The regex anchors to line-start and requires "[" immediately after the id,
  // so it won't match edge lines (which have "--" or "->" between the two ids).
  const typeMap = new Map<string, NodeType>();
  const attrRe = /^\s*"([^"]+)"\s*\[([^\]]*)\]/gm;
  let m: RegExpExecArray | null;
  while ((m = attrRe.exec(src)) !== null) {
    const [, id, attrs] = m;
    const tm = attrs.match(/import_type="([^"]+)"/);
    if (tm) {
      const t = goTypeToNodeType[tm[1]];
      if (t) typeMap.set(id, t);
    }
  }

  // Second pass: edges
  const edgeRe = /"([^"]+)"\s*(?:--|->)\s*"([^"]+)"/g;
  while ((m = edgeRe.exec(src)) !== null) {
    const [, a, b] = m;
    const key = [a, b].sort().join('\x00');
    if (!edgeSet.has(key)) {
      edgeSet.add(key);
      if (!nodes.has(a)) nodes.set(a, makeNode(a, typeMap));
      if (!nodes.has(b)) nodes.set(b, makeNode(b, typeMap));
      links.push({ source: a, target: b });
    }
  }

  // Third pass: isolated vertex declarations not seen in any edge
  const nodeRe = /^\s*"([^"]+)"\s*\[/gm;
  while ((m = nodeRe.exec(src)) !== null) {
    const id = m[1];
    if (!nodes.has(id)) nodes.set(id, makeNode(id, typeMap));
  }

  return { nodes: [...nodes.values()], links };
}
