<script lang="ts">
  import { createEventDispatcher } from 'svelte';

  export let error: string | null = null;

  const dispatch = createEventDispatcher<{ load: File }>();
  let dragOver   = false;
  let fileInput: HTMLInputElement;

  function handleDrop(e: DragEvent) {
    e.preventDefault();
    dragOver = false;
    const file = e.dataTransfer?.files[0];
    if (file) dispatch('load', file);
  }

  function handleChange() {
    if (fileInput.files?.[0]) dispatch('load', fileInput.files[0]);
    fileInput.value = '';
  }
</script>

<!-- svelte-ignore a11y-no-noninteractive-element-interactions -->
<div
  class="drop-zone"
  class:drag-over={dragOver}
  role="button"
  tabindex="0"
  on:click={() => fileInput.click()}
  on:keydown={e => e.key === 'Enter' && fileInput.click()}
  on:dragover={e => { e.preventDefault(); dragOver = true; }}
  on:dragleave={() => dragOver = false}
  on:drop={handleDrop}
>
  <input
    type="file"
    accept=".gv,.dot"
    bind:this={fileInput}
    on:change={handleChange}
  />
  <h2>Drop a .gv file here</h2>
  <p>or click to browse · strict graph / digraph DOT format</p>
  {#if error}
    <p class="error">{error}</p>
  {/if}
</div>

<style>
  .drop-zone {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 12px;
    border: 2px dashed var(--drop-border);
    border-radius: 12px;
    margin: 48px;
    cursor: pointer;
    transition: border-color 0.15s, background 0.15s;
    user-select: none;
  }

  .drop-zone.drag-over {
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 5%, transparent);
  }

  input[type="file"] { display: none; }

  h2 {
    font-size: 1.375rem;
    font-weight: 600;
    color: var(--text-muted);
  }

  p {
    font-size: 0.8125rem;
    color: var(--text-dim);
  }

  .error {
    color: var(--error);
    font-size: 0.8125rem;
  }
</style>
