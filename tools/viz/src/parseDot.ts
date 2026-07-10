import type { NodeType, GraphNode, GraphData } from './types.js';

const goTypeToNodeType: Record<string, NodeType> = {
  local:      'local',
  dependency: 'vendor',
  stdlib:     'stdlib',
};

function unescapeGoString(s: string): string {
  return s.replace(/\\(n|t|r|\\|")/g, (_, c: string) =>
    c === 'n' ? '\n' : c === 't' ? '\t' : c === 'r' ? '\r' : c
  );
}

// Handles both quoted keys ("observation-answer"="val") and unquoted keys (import_type="val")
function extractAttr(attrs: string, name: string): string | undefined {
  const re = new RegExp(`(?:"${name}"|${name})="((?:[^"\\\\]|\\\\.)*)"`);
  const m = attrs.match(re);
  return m ? unescapeGoString(m[1]) : undefined;
}

function makeNode(
  id: string,
  typeMap: Map<string, NodeType>,
  attrsMap: Map<string, Record<string, string>>,
): GraphNode {
  const a = attrsMap.get(id) ?? {};
  return {
    id,
    label:     a['label'] ?? (id.split('/').pop() ?? id),
    type:      typeMap.get(id) ?? 'unknown',
    answer:  a['observation-answer'],
    edgeCases: a['observation-edgecases'],
  };
}

export function parseDOT(src: string): GraphData {
  const nodes   = new Map<string, GraphNode>();
  const edgeSet = new Set<string>();
  const links: GraphData['links'] = [];

  const typeMap    = new Map<string, NodeType>();
  const attrsMap   = new Map<string, Record<string, string>>();

  // First pass: collect vertex attributes.
  // Anchored to line start so attr values containing "--"/"-->" won't be misread as edges.
  // The bracket body skips over quoted strings so values containing "]" (e.g. "cli.Args[1]")
  // don't prematurely terminate the match.
  const attrRe = /^\s*"([^"]+)"\s*\[((?:"(?:[^"\\]|\\.)*"|[^\]"])*)\]/gm;
  let m: RegExpExecArray | null;
  while ((m = attrRe.exec(src)) !== null) {
    const [, id, attrs] = m;

    const importType = extractAttr(attrs, 'import_type');
    if (importType) {
      const mapped = goTypeToNodeType[importType];
      if (mapped) typeMap.set(id, mapped);
    }

    const collected: Record<string, string> = {};
    for (const key of ['label', 'observation-answer', 'observation-edgecases']) {
      const val = extractAttr(attrs, key);
      if (val !== undefined) collected[key] = val;
    }
    if (Object.keys(collected).length > 0) attrsMap.set(id, collected);
  }

  // Second pass: edges
  const edgeRe = /^\s*"([^"]+)"\s*(?:--|->)\s*"([^"]+)"(?:\s*\[((?:"(?:[^"\\]|\\.)*"|[^\]"])*)\])?/gm;
  while ((m = edgeRe.exec(src)) !== null) {
    const [, a, b, edgeAttrs] = m;
    const key = [a, b].sort().join('\x00');
    if (!edgeSet.has(key)) {
      edgeSet.add(key);
      if (!nodes.has(a)) nodes.set(a, makeNode(a, typeMap, attrsMap));
      if (!nodes.has(b)) nodes.set(b, makeNode(b, typeMap, attrsMap));
      links.push({
        source:    a,
        target:    b,
        answer:  edgeAttrs ? extractAttr(edgeAttrs, 'observation-answer') : undefined,
        edgeCases: edgeAttrs ? extractAttr(edgeAttrs, 'observation-edgecases') : undefined,
      });
    }
  }

  // Third pass: isolated vertices not seen in any edge
  const nodeRe = /^\s*"([^"]+)"\s*\[/gm;
  while ((m = nodeRe.exec(src)) !== null) {
    const id = m[1];
    if (!nodes.has(id)) nodes.set(id, makeNode(id, typeMap, attrsMap));
  }

  return { nodes: [...nodes.values()], links };
}
