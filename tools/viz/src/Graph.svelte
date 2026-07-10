<script lang="ts">
  import { onMount, createEventDispatcher } from 'svelte';
  import * as d3 from 'd3';
  import type { Selection, D3DragEvent, D3ZoomEvent } from 'd3';
  import type { GraphData, GraphNode, SimNode, GraphLink, HoveredNode } from './types.js';

  export let data: GraphData;
  const dispatch = createEventDispatcher();

  const RADIUS: Record<string, number> = { local: 9, vendor: 5, stdlib: 4, unknown: 6 };
  const ALL_TYPES = ['local', 'vendor', 'stdlib', 'unknown'] as const;

  function getTheme() {
    const dark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    return dark
      ? {
          node:   { local: '#58a6ff', vendor: '#f0883e', stdlib: '#3d444d', unknown: '#666' } as Record<string, string>,
          link:   { local: '#388bfd', vendor: '#bd561d', stdlib: '#30363d', unknown: '#555' } as Record<string, string>,
          path:   '#f0e68c',
          label:  '#c9d1d9',
          stroke: (t: string) => d3.color(({ local: '#58a6ff', vendor: '#f0883e', stdlib: '#3d444d', unknown: '#666' } as Record<string, string>)[t])!.darker(1).toString(),
        }
      : {
          node:   { local: '#0969da', vendor: '#bc4c00', stdlib: '#8c959f', unknown: '#aaa' } as Record<string, string>,
          link:   { local: '#0550ae', vendor: '#953800', stdlib: '#d0d7de', unknown: '#ccc' } as Record<string, string>,
          path:   '#d4a017',
          label:  '#24292f',
          stroke: (t: string) => d3.color(({ local: '#0969da', vendor: '#bc4c00', stdlib: '#8c959f', unknown: '#aaa' } as Record<string, string>)[t])!.darker(0.5).toString(),
        };
  }

  // ---- persistent UI state ----
  let canvasEl: HTMLDivElement;
  let hoveredNode: HoveredNode | null = null;
  let hoveredPathEdges = new Set<string>();
  let searchQuery = '';
  let hideUnmatched = false;
  let typeFilter = new Set<string>(['local', 'vendor', 'stdlib', 'unknown']);
  let depthLimit: number | null = null;

  interface SelectionEntry {
    kind: 'node' | 'edge';
    key: string;
    label: string;
    sub: string;
    answer?: string;
    edgeCases?: string;
    deg?: number;
    pathToRoot?: string[] | null;
  }
  let selectedNodeIds = new Set<string>();
  let selectedEdgeKeys = new Set<string>();
  let selectionList: SelectionEntry[] = [];

  // D3 selections (set by drawGraph)
  let edgeSel:         Selection<SVGLineElement, GraphLink, SVGGElement, unknown>;
  let nodeSel:         Selection<SVGGElement,    SimNode,   SVGGElement, unknown>;
  let labelSel:        Selection<SVGTextElement, SimNode,   SVGGElement, unknown>;
  let vendorLabelsSel: Selection<SVGTextElement, SimNode,   SVGGElement, unknown>;
  let currentScale = 1;
  let simulation: d3.Simulation<SimNode, GraphLink>;

  // Graph-data state populated by drawGraph
  let theme: ReturnType<typeof getTheme>;
  let nodeMap: Map<string, SimNode> = new Map();
  let degree: Map<string, number> = new Map();
  let neighbors: Map<string, Set<string>> = new Map();
  let rootId: string | null = null;
  let defaultEdgeStroke: (l: GraphLink) => string = () => '#888';
  let defaultEdgeWidth:  (l: GraphLink) => number = () => 0.75;

  function linkEndId(ep: string | SimNode): string {
    return typeof ep === 'string' ? ep : ep.id;
  }
  function linkEndType(ep: string | SimNode): string {
    return typeof ep === 'string' ? (nodeMap.get(ep)?.type ?? 'stdlib') : ep.type;
  }

  $: counts = Object.entries(
    data.nodes.reduce<Record<string, number>>((acc, n) => {
      acc[n.type] = (acc[n.type] ?? 0) + 1;
      return acc;
    }, {})
  );

  $: applyVisuals(searchQuery, hideUnmatched, hoveredNode, hoveredPathEdges, selectedNodeIds, selectedEdgeKeys, typeFilter, depthLimit);

  function bfsWithinDepth(focalIds: Set<string>, limit: number): Set<string> {
    const visible = new Set<string>(focalIds);
    let frontier = [...focalIds];
    for (let hop = 0; hop < limit; hop++) {
      const next: string[] = [];
      for (const id of frontier) {
        for (const nbr of (neighbors.get(id) ?? [])) {
          if (!visible.has(nbr)) { visible.add(nbr); next.push(nbr); }
        }
      }
      frontier = next;
      if (frontier.length === 0) break;
    }
    return visible;
  }

  function applyVisuals(
    query: string,
    hide: boolean,
    hovered: HoveredNode | null,
    pathEdges: Set<string>,
    selNodes: Set<string>,
    selEdges: Set<string>,
    filter: Set<string>,
    depth: number | null,
  ) {
    if (!edgeSel || !nodeSel || !labelSel) return;

    const q = query.trim().toLowerCase();
    const hasHover = hovered !== null;
    const hasSelection = selNodes.size > 0 || selEdges.size > 0;

    const focalIds = selNodes.size > 0 ? selNodes : (rootId ? new Set([rootId]) : new Set<string>());
    const depthVisible = depth !== null && focalIds.size > 0 ? bfsWithinDepth(focalIds, depth) : null;

    nodeSel.attr('display', (n: SimNode) => {
      if (!filter.has(n.type)) return 'none';
      if (depthVisible && !depthVisible.has(n.id)) return 'none';
      return null;
    });
    edgeSel.attr('display', (l: GraphLink) => {
      const s = linkEndType(l.source), t = linkEndType(l.target);
      if (!filter.has(s) || !filter.has(t)) return 'none';
      if (depthVisible && !depthVisible.has(linkEndId(l.source))) return 'none';
      return null;
    });

    if (!hasHover && !hasSelection && !q) {
      nodeSel.selectAll('circle').attr('opacity', 1).attr('stroke-width', 1.5);
      labelSel.attr('opacity', (n: SimNode) => n.type === 'local' ? 1 : currentScale > 2 ? 1 : 0);
      edgeSel.attr('opacity', 0.45).attr('stroke', defaultEdgeStroke).attr('stroke-width', defaultEdgeWidth);
      return;
    }

    const activeNodes = new Set<string>(selNodes);
    const activeEdges = new Set<string>(selEdges);

    if (hasHover) {
      const nb = neighbors.get(hovered!.id) ?? new Set<string>();
      activeNodes.add(hovered!.id);
      for (const nbr of nb) activeNodes.add(nbr);
      for (const k of pathEdges) activeEdges.add(k);
    }

    if (q) {
      nodeSel.each((n: SimNode) => {
        if (n.label.toLowerCase().includes(q) || n.id.toLowerCase().includes(q)) activeNodes.add(n.id);
      });
    }

    const nodeDim = hasHover ? 0.22 : 0.25;

    nodeSel.selectAll('circle')
      .attr('opacity',      (n: SimNode) => activeNodes.has(n.id) ? 1 : nodeDim)
      .attr('stroke-width', (n: SimNode) => selNodes.has(n.id) ? 3 : 1.5);
    labelSel.attr('opacity', (n: SimNode) => activeNodes.has(n.id) ? 1 : nodeDim * 0.5);

    edgeSel
      .attr('opacity', (l: GraphLink) => {
        const sid = linkEndId(l.source), tid = linkEndId(l.target);
        const key = [sid, tid].sort().join('\x00');
        if (activeEdges.has(key)) return 1;
        if (!hasHover && (activeNodes.has(sid) || activeNodes.has(tid))) return 0.45;
        if (hasHover) return 0.04;
        return hide && q ? 0 : 0.08;
      })
      .attr('stroke', (l: GraphLink) => {
        const sid = linkEndId(l.source), tid = linkEndId(l.target);
        const key = [sid, tid].sort().join('\x00');
        if (selEdges.has(key) || pathEdges.has(key)) return theme?.path ?? '#f0e68c';
        return defaultEdgeStroke(l);
      })
      .attr('stroke-width', (l: GraphLink) => {
        const sid = linkEndId(l.source), tid = linkEndId(l.target);
        const key = [sid, tid].sort().join('\x00');
        if (selEdges.has(key) || pathEdges.has(key)) return 3;
        return defaultEdgeWidth(l);
      });
  }

  function toggleNodeSelection(d: SimNode) {
    const next = new Set(selectedNodeIds);
    if (next.has(d.id)) {
      next.delete(d.id);
      selectionList = selectionList.filter(e => !(e.kind === 'node' && e.key === d.id));
    } else {
      next.add(d.id);
      const pathToRoot = rootId && d.id !== rootId ? bfsPath(d.id, rootId) : null;
      selectionList = [...selectionList, {
        kind: 'node', key: d.id, label: d.label, sub: d.type,
        answer: d.answer, edgeCases: d.edgeCases, deg: degree.get(d.id), pathToRoot,
      }];
    }
    selectedNodeIds = next;
  }

  function toggleEdgeSelection(l: GraphLink, key: string) {
    const next = new Set(selectedEdgeKeys);
    if (next.has(key)) {
      next.delete(key);
      selectionList = selectionList.filter(e => !(e.kind === 'edge' && e.key === key));
    } else {
      next.add(key);
      const sid = linkEndId(l.source), tid = linkEndId(l.target);
      selectionList = [...selectionList, {
        kind: 'edge', key,
        label: `${sid.split('/').pop()} → ${tid.split('/').pop()}`,
        sub: `${linkEndType(l.source)} / ${linkEndType(l.target)}`,
        answer: l.answer, edgeCases: l.edgeCases,
      }];
    }
    selectedEdgeKeys = next;
  }

  function removeSelection(kind: 'node' | 'edge', key: string) {
    selectionList = selectionList.filter(e => !(e.kind === kind && e.key === key));
    if (kind === 'node') { const s = new Set(selectedNodeIds); s.delete(key); selectedNodeIds = s; }
    else                 { const s = new Set(selectedEdgeKeys); s.delete(key); selectedEdgeKeys = s; }
  }

  function clearSelection() {
    selectedNodeIds = new Set();
    selectedEdgeKeys = new Set();
    selectionList = [];
  }

  onMount(() => {
    const mq = window.matchMedia('(prefers-color-scheme: dark)');
    const onThemeChange = () => drawGraph(data);
    mq.addEventListener('change', onThemeChange);
    drawGraph(data);
    return () => {
      simulation?.stop();
      mq.removeEventListener('change', onThemeChange);
    };
  });

  function bfsPath(startId: string, targetId: string): string[] | null {
    if (startId === targetId) return [startId];
    const queue: string[][] = [[startId]];
    const visited = new Set<string>([startId]);
    while (queue.length > 0) {
      const path = queue.shift()!;
      for (const nbr of (neighbors.get(path[path.length - 1]) ?? [])) {
        if (visited.has(nbr)) continue;
        const np = [...path, nbr];
        if (nbr === targetId) return np;
        visited.add(nbr);
        queue.push(np);
      }
    }
    return null;
  }

  function drawGraph({ nodes, links }: GraphData) {
    d3.select(canvasEl).selectAll('*').remove();
    currentScale = 1;
    clearSelection();

    theme = getTheme();
    const W = canvasEl.clientWidth;
    const H = canvasEl.clientHeight;

    const simNodes = nodes as SimNode[];
    const simLinks = links as GraphLink[];

    nodeMap = new Map<string, SimNode>(simNodes.map(n => [n.id, n]));
    degree  = new Map<string, number>(simNodes.map(n => [n.id, 0]));
    neighbors = new Map<string, Set<string>>(simNodes.map(n => [n.id, new Set<string>()]));

    simLinks.forEach(l => {
      const sid = l.source as string, tid = l.target as string;
      degree.set(sid, (degree.get(sid) ?? 0) + 1);
      degree.set(tid, (degree.get(tid) ?? 0) + 1);
      neighbors.get(sid)?.add(tid);
      neighbors.get(tid)?.add(sid);
    });

    graphSerial++;

    rootId = null;
    let rootDeg = -1;
    for (const pass of ['main', 'local', 'any'] as const) {
      for (const n of simNodes) {
        if (pass === 'main'  && !(n.type === 'local' && n.label === 'main.go')) continue;
        if (pass === 'local' && n.type !== 'local') continue;
        const d = neighbors.get(n.id)?.size ?? 0;
        if (d > rootDeg) { rootDeg = d; rootId = n.id; }
      }
      if (rootId) break;
    }

    const svg   = d3.select(canvasEl).append('svg');
    const zoomG = svg.append('g');

    simulation = d3.forceSimulation<SimNode, GraphLink>(simNodes)
      .force('link', d3.forceLink<SimNode, GraphLink>(simLinks)
        .id(d => d.id)
        .distance((d: GraphLink) => {
          const s = nodeMap.get(d.source as string)?.type ?? (d.source as SimNode).type;
          const t = nodeMap.get(d.target as string)?.type ?? (d.target as SimNode).type;
          return (s === 'local' && t === 'local') ? 120
               : (s === 'local' || t === 'local') ? 90 : 55;
        })
        .strength(0.6))
      .force('charge', d3.forceManyBody<SimNode>()
        .strength(d => d.type === 'local' ? -500 : -120)
        .distanceMax(400)
        .theta(1.2))
      .force('center',  d3.forceCenter(W / 2, H / 2).strength(0.08))
      .force('collide', d3.forceCollide<SimNode>(d => (RADIUS[d.type] ?? 4) + 3).strength(0.7))
      .alphaDecay(0.04);

    const linkSrcs = simLinks.map(l => l.source as SimNode);
    const linkTgts = simLinks.map(l => l.target as SimNode);
    const edgeKeys = simLinks.map((_, i) => [linkSrcs[i].id, linkTgts[i].id].sort().join('\x00'));
    const linkKeyMap = new Map<GraphLink, string>(simLinks.map((l, i) => [l, edgeKeys[i]]));

    defaultEdgeStroke = (l: GraphLink) => {
      const s = (l.source as SimNode).type, t = (l.target as SimNode).type;
      return (s === 'local' || t === 'local') ? theme.link.local
           : (s === 'vendor' || t === 'vendor') ? theme.link.vendor
           : theme.link.stdlib;
    };
    defaultEdgeWidth = (l: GraphLink) => {
      const s = (l.source as SimNode).type, t = (l.target as SimNode).type;
      return (s === 'local' && t === 'local') ? 1.5 : 0.75;
    };

    edgeSel = zoomG.append('g').selectAll<SVGLineElement, GraphLink>('line')
      .data(simLinks).join('line')
      .attr('stroke', (d: GraphLink) => {
        const s = linkEndType(d.source), t = linkEndType(d.target);
        return (s === 'local' || t === 'local') ? theme.link.local
             : (s === 'vendor' || t === 'vendor') ? theme.link.vendor
             : theme.link.stdlib;
      })
      .attr('opacity', 0.45)
      .attr('stroke-width', (d: GraphLink) => {
        const s = linkEndType(d.source), t = linkEndType(d.target);
        return (s === 'local' && t === 'local') ? 1.5 : 0.75;
      });

    // Transparent wide lines for edge click hit area (under nodes)
    const edgeHitSel = zoomG.append('g').selectAll<SVGLineElement, GraphLink>('line')
      .data(simLinks).join('line')
      .attr('stroke', 'transparent')
      .attr('stroke-width', 10)
      .style('cursor', 'pointer')
      .on('click', (event: MouseEvent, l: GraphLink) => {
        event.stopPropagation();
        toggleEdgeSelection(l, linkKeyMap.get(l) ?? '');
      });

    nodeSel = zoomG.append('g').selectAll<SVGGElement, SimNode>('g')
      .data(simNodes).join('g')
      .call(dragBehavior(simulation));

    nodeSel.append('circle')
      .attr('r', (d: SimNode) => {
        const base = RADIUS[d.type] ?? 4;
        return d.type === 'local' ? base + Math.min(Math.sqrt(degree.get(d.id) ?? 0), 5) : base;
      })
      .attr('fill',         (d: SimNode) => theme.node[d.type])
      .attr('stroke',       (d: SimNode) => theme.stroke(d.type))
      .attr('stroke-width', 1.5)
      .style('cursor', 'pointer');

    labelSel = nodeSel.filter((d: SimNode) => d.type !== 'stdlib')
      .append('text')
      .attr('x',                 (d: SimNode) => (RADIUS[d.type] ?? 4) + 4)
      .attr('dominant-baseline', 'middle')
      .attr('font-size',         '10px')
      .attr('fill',              theme.label)
      .attr('pointer-events',    'none')
      .attr('opacity',           (d: SimNode) => d.type === 'local' ? 1 : 0)
      .text((d: SimNode) => d.label) as Selection<SVGTextElement, SimNode, SVGGElement, unknown>;

    vendorLabelsSel = labelSel.filter((d: SimNode) => d.type === 'vendor');

    const lineEls    = edgeSel.nodes()    as SVGLineElement[];
    const hitLineEls = edgeHitSel.nodes() as SVGLineElement[];
    const nodeEls    = nodeSel.nodes()    as SVGGElement[];

    const svgEl = svg.node()!;
    const nodeTransforms: SVGTransform[] = nodeEls.map(el => {
      const t = svgEl.createSVGTransform();
      t.setTranslate(0, 0);
      el.transform.baseVal.appendItem(t);
      return t;
    });

    nodeSel
      .on('mouseover', (_event: MouseEvent, d: SimNode) => {
        const pathToRoot = rootId && d.id !== rootId ? bfsPath(d.id, rootId) : null;
        const pe = new Set<string>();
        if (pathToRoot) {
          for (let i = 0; i < pathToRoot.length - 1; i++)
            pe.add([pathToRoot[i], pathToRoot[i + 1]].sort().join('\x00'));
        }
        hoveredPathEdges = pe;
        hoveredNode = { ...d, deg: degree.get(d.id) ?? 0, pathToRoot };
      })
      .on('mouseout', () => {
        hoveredNode = null;
        hoveredPathEdges = new Set();
      })
      .on('click', (event: MouseEvent, d: SimNode) => {
        event.stopPropagation();
        toggleNodeSelection(d);
      });

    svg.on('click', () => clearSelection());

    let frame = 0;
    simulation.on('tick', () => {
      frame++;
      const a = simulation.alpha();
      if (a > 0.5 && frame % 3 !== 0) return;
      if (a > 0.15 && frame % 2 !== 0) return;

      for (let i = 0; i < lineEls.length; i++) {
        const sx = linkSrcs[i].x!, sy = linkSrcs[i].y!;
        const tx = linkTgts[i].x!, ty = linkTgts[i].y!;
        lineEls[i].x1.baseVal.value = sx; lineEls[i].y1.baseVal.value = sy;
        lineEls[i].x2.baseVal.value = tx; lineEls[i].y2.baseVal.value = ty;
        hitLineEls[i].x1.baseVal.value = sx; hitLineEls[i].y1.baseVal.value = sy;
        hitLineEls[i].x2.baseVal.value = tx; hitLineEls[i].y2.baseVal.value = ty;
      }
      for (let i = 0; i < nodeTransforms.length; i++) {
        nodeTransforms[i].setTranslate(simNodes[i].x!, simNodes[i].y!);
      }
    });

    svg.call(
      d3.zoom<SVGSVGElement, unknown>()
        .scaleExtent([0.05, 12])
        .on('zoom', (event: D3ZoomEvent<SVGSVGElement, unknown>) => {
          currentScale = event.transform.k;
          zoomG.attr('transform', event.transform.toString());
          if (!searchQuery.trim()) vendorLabelsSel.attr('opacity', currentScale > 2 ? 1 : 0);
        })
    );

    applyVisuals(searchQuery, hideUnmatched, hoveredNode, hoveredPathEdges, selectedNodeIds, selectedEdgeKeys, typeFilter, depthLimit);
  }

  const DEPTH_PRESETS = [1, 3, 5, 10] as const;
  $: depthCustomValue = depthLimit !== null && !(DEPTH_PRESETS as readonly number[]).includes(depthLimit) ? String(depthLimit) : '';

  let graphSerial = 0; // bumped each drawGraph so reactive max-depth recomputes on reload

  function computeMaxDepth(focalIds: Set<string>): number {
    if (focalIds.size === 0 || neighbors.size === 0) return 0;
    const visited = new Set<string>(focalIds);
    let frontier = [...focalIds];
    let depth = 0;
    while (frontier.length > 0) {
      const next: string[] = [];
      for (const id of frontier)
        for (const nbr of (neighbors.get(id) ?? []))
          if (!visited.has(nbr)) { visited.add(nbr); next.push(nbr); }
      if (next.length > 0) depth++;
      frontier = next;
    }
    return depth;
  }

  $: graphMaxDepth = (() => {
    if (!graphSerial) return Infinity;
    const focalIds = selectedNodeIds.size > 0 ? selectedNodeIds : (rootId ? new Set([rootId]) : null);
    return focalIds ? computeMaxDepth(focalIds) : Infinity;
  })();

  function setDepthPreset(d: number) {
    depthLimit = depthLimit === d ? null : d;
  }

  function onDepthCustomInput(e: Event) {
    const v = parseInt((e.target as HTMLInputElement).value);
    depthLimit = !isNaN(v) && v > 0 ? v : null;
  }

  function onTypeFilterChange(e: Event, t: string) {
    const next = new Set(typeFilter);
    if ((e.target as HTMLInputElement).checked) next.add(t); else next.delete(t);
    typeFilter = next;
  }

  function dragBehavior(sim: d3.Simulation<SimNode, GraphLink>) {
    return d3.drag<SVGGElement, SimNode>()
      .on('start', (event: D3DragEvent<SVGGElement, SimNode, SimNode>, d) => {
        if (!event.active) sim.alphaTarget(0.3).restart();
        d.fx = d.x; d.fy = d.y;
      })
      .on('drag', (event: D3DragEvent<SVGGElement, SimNode, SimNode>, d) => {
        d.fx = event.x; d.fy = event.y;
      })
      .on('end', (event: D3DragEvent<SVGGElement, SimNode, SimNode>, d) => {
        if (!event.active) sim.alphaTarget(0);
        d.fx = null; d.fy = null;
      });
  }
</script>

<div class="graph-view">
  <div class="canvas" bind:this={canvasEl}></div>

  <div class="panel stats">
    <div>{data.nodes.length} nodes · {data.links.length} edges</div>
    {#each counts as [type, count]}
      <div style="color: var(--color-{type})">{count} {type}</div>
    {/each}
  </div>

  <div class="panel search-bar">
    <div class="search-row">
      <input
        class="search-input"
        type="search"
        placeholder="filter nodes…"
        bind:value={searchQuery}
        on:keydown={e => e.key === 'Escape' && (searchQuery = '')}
      />
      <label class="hide-toggle">
        <input type="checkbox" bind:checked={hideUnmatched} />
        hide unmatched
      </label>
      <div class="type-filter">
        {#each ALL_TYPES as t}
          <label class="type-chip" class:active={typeFilter.has(t)}>
            <input
              type="checkbox"
              checked={typeFilter.has(t)}
              on:change={e => onTypeFilterChange(e, t)}
            />
            <span class="dot" style="background: var(--color-{t})"></span>{t}
          </label>
        {/each}
      </div>
    </div>
    <div class="depth-row">
      <span class="depth-label">depth</span>
      <div class="depth-seg">
        {#each DEPTH_PRESETS as d}
          <button class="depth-btn" class:active={depthLimit === d} disabled={d > graphMaxDepth} on:click={() => setDepthPreset(d)}>{d}</button>
        {/each}
      </div>
      <input
        class="depth-custom"
        type="number"
        min="1"
        placeholder="…"
        value={depthCustomValue}
        on:input={onDepthCustomInput}
      />
    </div>
  </div>

  {#if hoveredNode}
    <div class="panel tooltip">
      <div class="tip-label">{hoveredNode.label}</div>
      <div class="tip-type">{hoveredNode.type}</div>
      <div class="tip-path">{hoveredNode.id}</div>
      {#if hoveredNode.pathToRoot && hoveredNode.pathToRoot.length > 1}
        <div class="tip-chain">
          {[...hoveredNode.pathToRoot].reverse().map(id => id.split('/').pop()).join(' → ')}
        </div>
      {/if}
      <div class="tip-deg">{hoveredNode.deg} connection{hoveredNode.deg !== 1 ? 's' : ''}</div>
      {#if hoveredNode.answer}
        <div class="tip-observation">{hoveredNode.answer}</div>
      {/if}
      {#if hoveredNode.edgeCases}
        <div class="tip-edgecases"><span class="tip-edgecases-label">edge cases</span>{hoveredNode.edgeCases}</div>
      {/if}
    </div>
  {/if}

  {#if selectionList.length > 0}
    <div class="panel selection-panel">
      <div class="sel-header">
        <span>{selectionList.length} selected</span>
        <button class="sel-clear" on:click={clearSelection}>clear all</button>
      </div>
      {#each selectionList as item (item.key)}
        <div class="sel-item">
          <div class="sel-item-row">
            <span class="sel-kind">{item.kind}</span>
            <span class="sel-label">{item.label}</span>
            <button class="sel-remove" on:click={() => removeSelection(item.kind, item.key)}>×</button>
          </div>
          <div class="sel-sub">{item.sub}</div>
          {#if item.deg !== undefined}
            <div class="sel-detail">{item.deg} connection{item.deg !== 1 ? 's' : ''}</div>
          {/if}
          {#if item.pathToRoot && item.pathToRoot.length > 1}
            <div class="sel-path">
              {[...item.pathToRoot].reverse().map(id => id.split('/').pop()).join(' → ')}
            </div>
          {/if}
          {#if item.answer}
            <div class="sel-observation">{item.answer}</div>
          {/if}
          {#if item.edgeCases}
            <div class="sel-edgecases"><span class="sel-edgecases-label">edge cases</span>{item.edgeCases}</div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}

  <button class="panel reload-btn" on:click={() => dispatch('reset')}>load new file</button>
</div>

<style>
  .graph-view { position: relative; flex: 1; min-height: 0; }

  .canvas { width: 100%; height: 100%; }
  .canvas :global(svg) { width: 100%; height: 100%; display: block; }

  .panel {
    position: absolute;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 10px 14px;
    font-size: 11.5px;
    backdrop-filter: blur(4px);
  }

  .stats { top: 16px; left: 16px; color: var(--text-muted); line-height: 1.7; }

  .search-bar {
    top: 16px; left: 50%; transform: translateX(-50%);
    display: flex; flex-direction: column; gap: 7px; padding: 8px 12px; white-space: nowrap;
  }
  .search-row { display: flex; align-items: center; gap: 10px; }

  .search-input {
    background: none; border: none; outline: none;
    color: var(--text); font-family: inherit; font-size: 12px; width: 170px;
  }
  .search-input::placeholder { color: var(--text-dim); }

  .hide-toggle {
    display: flex; align-items: center; gap: 5px;
    color: var(--text-muted); font-size: 11px; cursor: pointer; user-select: none;
    border-left: 1px solid var(--border); padding-left: 10px;
  }

  .type-filter {
    display: flex; align-items: center; gap: 6px;
    border-left: 1px solid var(--border); padding-left: 10px;
  }
  .type-chip {
    display: flex; align-items: center; gap: 4px;
    font-size: 11px; color: var(--text-muted); cursor: pointer;
    user-select: none; opacity: 0.4; transition: opacity 0.15s;
  }
  .type-chip.active { opacity: 1; }
  .type-chip input { display: none; }

  .depth-row {
    display: flex; align-items: center; gap: 8px;
    border-top: 1px solid var(--border); padding-top: 7px;
  }
  .depth-label { font-size: 11px; color: var(--text-muted); }
  .depth-seg { display: flex; border: 1px solid var(--border); border-radius: 5px; overflow: hidden; }
  .depth-btn {
    background: none; border: none; border-right: 1px solid var(--border);
    padding: 2px 8px; font-size: 11px; font-family: inherit;
    color: var(--text-muted); cursor: pointer; transition: background 0.1s, color 0.1s;
  }
  .depth-btn:last-child { border-right: none; }
  .depth-btn.active { background: var(--accent); color: #fff; }
  .depth-btn:not(.active):hover:not(:disabled) { background: var(--border); }
  .depth-btn:disabled { opacity: 0.25; cursor: not-allowed; }
  .depth-custom {
    width: 44px; background: none; border: 1px solid var(--border); border-radius: 5px;
    padding: 2px 6px; font-size: 11px; font-family: inherit; color: var(--text);
    outline: none; text-align: center;
  }
  .depth-custom::placeholder { color: var(--text-dim); }
  .depth-custom::-webkit-inner-spin-button,
  .depth-custom::-webkit-outer-spin-button { -webkit-appearance: none; }

  .tooltip {
    top: 16px; right: 16px; max-width: 340px; word-break: break-all; line-height: 1.6;
  }
  .tip-label   { font-size: 13px; font-weight: 600; color: var(--text-strong); }
  .tip-type    { margin-top: 2px; color: var(--text-muted); }
  .tip-path    { margin-top: 6px; font-size: 10px; color: var(--text-dim); }
  .tip-chain   { margin-top: 6px; font-size: 10px; color: var(--accent); word-break: break-word; line-height: 1.5; }
  .tip-deg     { margin-top: 4px; color: var(--text-muted); }
  .tip-observation {
    margin-top: 8px; padding-top: 8px; border-top: 1px solid var(--border);
    font-size: 11px; color: var(--text); white-space: pre-wrap; word-break: break-word;
    max-height: 160px; overflow-y: auto; line-height: 1.55;
  }
  .tip-edgecases {
    margin-top: 6px; font-size: 10px; color: var(--text-muted);
    white-space: pre-wrap; word-break: break-word; max-height: 100px; overflow-y: auto; line-height: 1.5;
  }
  .tip-edgecases-label {
    display: block; font-size: 9px; text-transform: uppercase; letter-spacing: 0.06em;
    color: var(--text-dim); margin-bottom: 2px;
  }

  .selection-panel {
    bottom: 60px; right: 16px; max-width: 300px; max-height: 45vh;
    overflow-y: auto; padding: 8px 12px;
  }
  .sel-header {
    display: flex; justify-content: space-between; align-items: center;
    font-size: 11px; color: var(--text-muted); margin-bottom: 6px;
    padding-bottom: 6px; border-bottom: 1px solid var(--border);
  }
  .sel-clear {
    background: none; border: none; cursor: pointer; padding: 0;
    color: var(--text-muted); font-size: 11px; font-family: inherit;
  }
  .sel-clear:hover { color: var(--text); }
  .sel-item {
    padding: 6px 0; border-bottom: 1px solid var(--border); line-height: 1.5;
  }
  .sel-item:last-child { border-bottom: none; }
  .sel-item-row { display: flex; align-items: baseline; gap: 5px; }
  .sel-kind {
    font-size: 9px; text-transform: uppercase; letter-spacing: 0.05em;
    color: var(--text-dim); flex-shrink: 0;
  }
  .sel-label { font-size: 12px; font-weight: 600; color: var(--text-strong); flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .sel-remove {
    background: none; border: none; cursor: pointer; color: var(--text-muted);
    font-size: 14px; line-height: 1; padding: 0; flex-shrink: 0;
  }
  .sel-remove:hover { color: var(--text); }
  .sel-sub          { font-size: 10px; color: var(--text-muted); }
  .sel-detail       { font-size: 10px; color: var(--text-muted); }
  .sel-path         { font-size: 10px; color: var(--accent); word-break: break-word; }
  .sel-observation  {
    font-size: 10px; color: var(--text); white-space: pre-wrap; word-break: break-word;
    margin-top: 4px; max-height: 100px; overflow-y: auto; line-height: 1.5;
  }
  .sel-edgecases {
    font-size: 10px; color: var(--text-muted); white-space: pre-wrap; word-break: break-word;
    margin-top: 3px; max-height: 80px; overflow-y: auto; line-height: 1.5;
  }
  .sel-edgecases-label {
    display: inline; font-size: 9px; text-transform: uppercase; letter-spacing: 0.06em;
    color: var(--text-dim); margin-right: 4px;
  }

  .dot { display: inline-block; width: 9px; height: 9px; border-radius: 50%; flex-shrink: 0; }

  .reload-btn {
    bottom: 16px; right: 16px; cursor: pointer;
    color: var(--accent); font-family: inherit; background: var(--surface);
    transition: border-color 0.15s;
  }
  .reload-btn:hover { border-color: var(--accent); }
</style>
