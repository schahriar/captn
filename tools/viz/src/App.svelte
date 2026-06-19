<script lang="ts">
  import DropZone from './DropZone.svelte';
  import Graph from './Graph.svelte';
  import { parseDOT } from './parseDot.js';
  import type { GraphData } from './types.js';

  let graphData:  GraphData | null = null;
  let parseError: string  | null = null;

  function handleLoad(file: File) {
    parseError = null;
    const reader = new FileReader();
    reader.onload = e => {
      const data = parseDOT((e.target as FileReader).result as string);
      if (data.nodes.length === 0) {
        parseError = 'No nodes found — is this a valid DOT file?';
      } else {
        graphData = data;
      }
    };
    reader.onerror = () => { parseError = 'Failed to read file.'; };
    reader.readAsText(file);
  }

  function handleReset() {
    graphData  = null;
    parseError = null;
  }
</script>

{#if graphData}
  <Graph data={graphData} on:reset={handleReset} />
{:else}
  <DropZone error={parseError} on:load={e => handleLoad(e.detail)} />
{/if}
