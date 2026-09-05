<script lang="ts">
  import { courses, navigate, route, selectedCourseID, unreadAnnouncements, unseenCount, upcomingCount } from '../stores';
  import type { Route } from '../stores';
  import Icon from './Icon.svelte';
  import Pet from './Pet.svelte';
  import SyncPill from './SyncPill.svelte';
  import { petEnabled } from '../pet';

  const NAV: Array<{ id: Route; label: string; icon: string }> = [
    { id: 'home', label: 'Home', icon: 'home' },
    { id: 'files', label: 'Files', icon: 'files' },
    { id: 'whatsnew', label: "What's new", icon: 'dot' },
    { id: 'deadlines', label: 'Deadlines', icon: 'deadlines' },
    { id: 'announcements', label: 'Announcements', icon: 'announcements' },
    { id: 'grades', label: 'Grades', icon: 'grades' },
    { id: 'study', label: 'Study', icon: 'layers' },
    { id: 'settings', label: 'Settings', icon: 'settings' },
    { id: 'arcade', label: 'Play', icon: 'gamepad' },
  ];

  function badgeFor(id: Route): number {
    if (id === 'deadlines') return $upcomingCount;
    if (id === 'announcements') return $unreadAnnouncements;
    if (id === 'whatsnew') return $unseenCount;
    return 0;
  }

  function pickCourse(id: number) {
    selectedCourseID.set($selectedCourseID === id ? 0 : id);
    navigate('files');
  }
</script>

<aside class="sidebar">
  <div class="brand">
    <span class="mark" aria-hidden="true"></span>
    <span class="word">NUSSync</span>
  </div>

  <nav class="nav">
    {#each NAV as item (item.id)}
      {@const badge = badgeFor(item.id)}
      <button
        class="nav-item"
        class:active={$route === item.id}
        onclick={() => navigate(item.id)}
        aria-current={$route === item.id ? 'page' : undefined}
      >
        <Icon name={item.icon} size={15} />
        <span class="nav-label">{item.label}</span>
        {#if badge > 0}<span class="badge">{badge}</span>{/if}
      </button>
    {/each}
  </nav>

  <div class="courses">
    <div class="courses-head">
      <span class="section-title">Courses</span>
      {#if $selectedCourseID !== 0}
        <button class="clear" onclick={() => selectedCourseID.set(0)}>Clear</button>
      {/if}
    </div>
    <div class="course-list">
      {#each $courses as c (c.ID)}
        <button
          class="course"
          class:active={$selectedCourseID === c.ID}
          class:off={!c.Enabled}
          onclick={() => pickCourse(c.ID)}
          title="{c.Code} — {c.Name}"
        >
          <span class="dot" style="background:{c.Color}"></span>
          <span class="code truncate">{c.Code}</span>
          <span class="count">{c.FileCount}</span>
        </button>
      {/each}
      {#if $courses.length === 0}
        <div class="no-courses faint">No courses yet</div>
      {/if}
    </div>
  </div>

  {#if $petEnabled}
    <Pet />
  {/if}

  <SyncPill />
</aside>

<style>
  .sidebar {
    width: var(--sidebar-w);
    flex: none;
    display: flex;
    flex-direction: column;
    min-height: 0;
    background: var(--bg-sidebar);
    border-right: 1px solid var(--border);
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 8px;
    height: var(--topbar-h);
    padding: 0 14px;
    flex: none;
  }

  .mark {
    width: 15px;
    height: 15px;
    border-radius: 5px;
    background: var(--accent);
    box-shadow: inset 0 0 0 3.5px var(--bg-sidebar);
  }

  .word {
    font-size: 13.5px;
    font-weight: 650;
    letter-spacing: -0.01em;
  }

  .nav {
    display: flex;
    flex-direction: column;
    gap: 1px;
    padding: 4px 8px 10px;
    flex: none;
  }

  .nav-item {
    display: flex;
    align-items: center;
    gap: 9px;
    width: 100%;
    height: 30px;
    padding: 0 8px;
    border-radius: var(--radius-sm);
    color: var(--text-muted);
    font-size: 13px;
    font-weight: 500;
    transition: background var(--t), color var(--t);
  }

  .nav-item:hover {
    background: var(--bg-hover);
    color: var(--text);
  }

  .nav-item.active {
    background: var(--bg-active);
    color: var(--accent-text);
    font-weight: 560;
  }

  .nav-label {
    flex: 1;
    text-align: left;
  }

  .badge {
    min-width: 17px;
    height: 17px;
    padding: 0 5px;
    border-radius: 999px;
    background: var(--bg-subtle);
    color: var(--text-muted);
    font-size: 10.5px;
    font-weight: 600;
    line-height: 17px;
    text-align: center;
    font-variant-numeric: tabular-nums;
  }

  .nav-item.active .badge {
    background: var(--accent);
    color: #fff;
  }

  .courses {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    padding: 0 8px;
  }

  .courses-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 4px 8px 6px;
  }

  .clear {
    font-size: 11px;
    color: var(--accent-text);
    font-weight: 550;
  }

  .clear:hover {
    text-decoration: underline;
  }

  .course-list {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 1px;
    padding-bottom: 10px;
  }

  .course {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    height: 27px;
    padding: 0 8px;
    border-radius: var(--radius-sm);
    color: var(--text-muted);
    font-size: 12.5px;
    transition: background var(--t), color var(--t);
  }

  .course:hover {
    background: var(--bg-hover);
    color: var(--text);
  }

  .course.active {
    background: var(--bg-active);
    color: var(--accent-text);
    font-weight: 560;
  }

  .course.off {
    opacity: 0.45;
  }

  .code {
    flex: 1;
    text-align: left;
  }

  .count {
    font-size: 11px;
    color: var(--text-faint);
    font-variant-numeric: tabular-nums;
  }

  .no-courses {
    padding: 8px;
    font-size: 12px;
  }
</style>
