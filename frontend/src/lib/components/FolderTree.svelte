<script lang="ts">
  import type { FileNode } from '../types';
  import Icon from './Icon.svelte';
  import Self from './FolderTree.svelte';

  interface Props {
    nodes: FileNode[];
    depth?: number;
    expanded: Set<string>;
    selected: string;
    onToggle: (key: string) => void;
    onSelect: (node: FileNode) => void;
    keyOf: (node: FileNode) => string;
  }

  let { nodes, depth = 0, expanded, selected, onToggle, onSelect, keyOf }: Props = $props();

  const dirs = $derived(nodes.filter((n) => n.IsDir));

  function childCount(n: FileNode): number {
    let count = 0;
    const walk = (list: FileNode[]) => {
      for (const c of list) {
        if (c.IsDir) walk(c.Children ?? []);
        else count++;
      }
    };
    walk(n.Children ?? []);
    return count;
  }
</script>

{#each dirs as node (keyOf(node))}
  {@const key = keyOf(node)}
  {@const isOpen = expanded.has(key)}
  {@const hasSubdirs = (node.Children ?? []).some((c) => c.IsDir)}
  <div class="node">
    <button
      class="row"
      class:sel={selected === key}
      style="padding-left:{6 + depth * 13}px"
      onclick={() => onSelect(node)}
    >
      <span
        class="twist"
        class:hidden={!hasSubdirs}
        role="button"
        tabindex="-1"
        onclick={(e) => {
          e.stopPropagation();
          onToggle(key);
        }}
        onkeydown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.stopPropagation();
            e.preventDefault();
            onToggle(key);
          }
        }}
      >
        <Icon name={isOpen ? 'chevronDown' : 'chevronRight'} size={12} />
      </span>
      <span class="ico"><Icon name={isOpen ? 'folderOpen' : 'folder'} size={14} /></span>
      <span class="name truncate">{node.Name}</span>
      <span class="n">{childCount(node)}</span>
    </button>

    {#if isOpen && hasSubdirs}
      <Self
        nodes={node.Children ?? []}
        depth={depth + 1}
        {expanded}
        {selected}
        {onToggle}
        {onSelect}
        {keyOf}
      />
    {/if}
  </div>
{/each}

<style>
  .row {
    display: flex;
    align-items: center;
    gap: 5px;
    width: 100%;
    height: 27px;
    padding-right: 8px;
    border-radius: var(--radius-sm);
    color: var(--text-muted);
    font-size: 12.5px;
    text-align: left;
    transition: background var(--t), color var(--t);
  }

  .row:hover {
    background: var(--bg-hover);
    color: var(--text);
  }

  .row.sel {
    background: var(--bg-active);
    color: var(--accent-text);
    font-weight: 550;
  }

  .twist {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 15px;
    height: 15px;
    border-radius: 3px;
    color: var(--text-faint);
    flex: none;
    transition: background var(--t);
  }

  .twist:hover {
    background: var(--bg-hover);
    color: var(--text);
  }

  .twist.hidden {
    visibility: hidden;
  }

  .ico {
    display: flex;
    color: var(--text-faint);
    flex: none;
  }

  .row.sel .ico {
    color: var(--accent-text);
  }

  .name {
    flex: 1;
  }

  .n {
    font-size: 10.5px;
    color: var(--text-faint);
    font-variant-numeric: tabular-nums;
  }
</style>
