import { type KeyboardEvent, type ReactNode, useCallback, useState } from 'react';
import { describeError } from '../../api/errors';
import type { DirSummary, Entry } from '../../api/types';
import { formatBytes, formatDateTime, formatMode } from '../../lib/format';
import { shownEntries } from '../../lib/listing';
import type { PaneId } from '../../stores/paneTypes';
import { type PaneState, usePanes } from '../../stores/panes';
import { useUi } from '../../stores/ui';
import { Icon } from '../ui/Icon';
import controls from '../ui/controls.module.css';
import styles from './PreviewPanel.module.css';
import { useHighlighted } from './useHighlighted';
import { type Loaded, usePreview } from './usePreview';

function cursorEntry(pane: PaneState): Entry | null {
  const { cursor } = pane.selection;
  if (cursor === null) return null;
  return shownEntries(pane.listing).find((e) => e.path === cursor) ?? null;
}

function focusSource(paneId: PaneId): void {
  document
    .querySelector<HTMLElement>(`[data-pane="${paneId}"] [data-filelist]`)
    ?.focus({ preventScroll: true });
}

function closePreview(paneId: PaneId): void {
  useUi.getState().closePreview();
  focusSource(paneId);
}

/** Quick preview of `sourcePane`'s cursor entry, shown in place of the other pane. */
export function PreviewPanel({ sourcePane }: { sourcePane: PaneId }) {
  const connId = usePanes((s) => s.panes[sourcePane].connId);
  const entry = usePanes((s) => cursorEntry(s.panes[sourcePane]));
  const state = usePreview(connId, entry);
  const [wrap, setWrap] = useState(true);
  const isText = state.status === 'ready' && state.content.kind === 'text';

  // Space, Escape and Tab mirror the list: close, close, back to the list.
  const onKeyDown = (event: KeyboardEvent) => {
    if (event.metaKey || event.ctrlKey || event.altKey) return;
    if (event.key === 'Escape' || (event.key === ' ' && event.target === event.currentTarget)) {
      event.preventDefault();
      closePreview(sourcePane);
    } else if (event.key === 'Tab') {
      event.preventDefault();
      focusSource(sourcePane);
    }
  };

  return (
    <section
      className={styles.panel}
      aria-label="Preview"
      data-preview=""
      data-side={sourcePane === 'left' ? 'right' : 'left'}
    >
      <header className={styles.header}>
        <Icon name={entry?.isDir ? 'folder' : 'file'} className={styles.icon} />
        <h2 className={styles.title} title={entry?.path}>
          {entry?.name ?? 'Preview'}
        </h2>
        {isText && (
          <button
            type="button"
            className={controls.button}
            aria-pressed={wrap}
            onClick={() => setWrap(!wrap)}
          >
            Wrap
          </button>
        )}
        <button
          type="button"
          className={controls.iconButton}
          aria-label="Close preview"
          title="Close preview (Space)"
          onClick={() => closePreview(sourcePane)}
        >
          <Icon name="close" />
        </button>
      </header>
      <div
        className={styles.body}
        tabIndex={0}
        data-preview-body=""
        aria-busy={state.status === 'loading'}
        onKeyDown={onKeyDown}
      >
        {state.status === 'empty' && <Message>No item selected</Message>}
        {state.status === 'loading' && <Message muted>Loading…</Message>}
        {state.status === 'error' && <Message error>{describeError(state.error)}</Message>}
        {state.status === 'ready' && entry && (
          <Content content={state.content} entry={entry} wrap={wrap} />
        )}
      </div>
      {entry && <Metadata entry={entry} />}
    </section>
  );
}

function Message({
  children,
  muted = false,
  error = false,
}: {
  children: ReactNode;
  muted?: boolean;
  error?: boolean;
}) {
  const tone = error ? styles.error : muted ? styles.muted : '';
  return <p className={`${styles.message} ${tone}`}>{children}</p>;
}

function Content({ content, entry, wrap }: { content: Loaded; entry: Entry; wrap: boolean }) {
  switch (content.kind) {
    case 'dir':
      return <Summary summary={content.summary} />;
    case 'text':
      return (
        <>
          {content.truncated && (
            <p className={styles.notice}>
              Showing the first {formatBytes(content.bytes)} of {formatBytes(entry.size)}
            </p>
          )}
          <Text content={content} name={entry.name} wrap={wrap} />
        </>
      );
    case 'image':
      return <ImagePreview blob={content.blob} name={entry.name} />;
    case 'pdf':
      // The webview's PDF viewer refuses to run in a sandboxed frame; the route serves only the PDF.
      // oxlint-disable-next-line react/iframe-missing-sandbox
      return <iframe className={styles.pdf} src={content.url} title={entry.name} />;
    case 'none':
      return (
        <Message>
          {content.reason === 'too_large'
            ? `File too large to preview (${formatBytes(entry.size)})`
            : 'No preview available'}
        </Message>
      );
    default: {
      const unreachable: never = content;
      return unreachable;
    }
  }
}

function Text({
  content,
  name,
  wrap,
}: {
  content: Extract<Loaded, { kind: 'text' }>;
  name: string;
  wrap: boolean;
}) {
  const html = useHighlighted(name, content.text, content.bytes);
  return (
    <pre className={wrap ? `${styles.text} ${styles.wrap}` : styles.text}>
      {html === null ? (
        content.text
      ) : (
        // highlight.js output: it escapes the file's text, which is never set as markup itself.
        <code dangerouslySetInnerHTML={{ __html: html }} />
      )}
    </pre>
  );
}

function ImagePreview({ blob, name }: { blob: Blob; name: string }) {
  const [brokenBlob, setBrokenBlob] = useState<Blob | null>(null);
  // The object URL lives exactly as long as the <img> showing it.
  const attach = useCallback(
    (img: HTMLImageElement | null) => {
      if (img === null) return;
      const url = URL.createObjectURL(blob);
      img.src = url;
      return () => URL.revokeObjectURL(url);
    },
    [blob]
  );

  if (brokenBlob === blob) return <Message>No preview available</Message>;
  // An <img> never runs an SVG's scripts.
  return (
    <img ref={attach} className={styles.image} alt={name} onError={() => setBrokenBlob(blob)} />
  );
}

function Summary({ summary }: { summary: DirSummary }) {
  const items = summary.dirs + summary.files;
  const count = summary.partial
    ? `More than ${items.toLocaleString()} items`
    : `${items.toLocaleString()} ${items === 1 ? 'item' : 'items'}`;
  return (
    <div className={styles.summary}>
      <Icon name="folder" size={48} className={styles.summaryIcon} />
      <p className={styles.count}>{count}</p>
      <p className={styles.detail}>
        {plural(summary.dirs, 'folder')}, {plural(summary.files, 'file')}
      </p>
      {!summary.partial && summary.files > 0 && (
        <p className={styles.detail}>
          {formatBytes(summary.size)} in files here, not counting subfolders
        </p>
      )}
    </div>
  );
}

function plural(n: number, noun: string): string {
  return `${n.toLocaleString()} ${noun}${n === 1 ? '' : 's'}`;
}

function Metadata({ entry }: { entry: Entry }) {
  const isLink = entry.link !== undefined && entry.link !== '';
  return (
    <dl className={styles.meta}>
      <dt>Kind</dt>
      <dd>{isLink ? 'Symbolic link' : entry.isDir ? 'Folder' : 'File'}</dd>
      {!entry.isDir && (
        <>
          <dt>Size</dt>
          <dd>
            {formatBytes(entry.size)}
            {entry.size >= 1024 && ` (${entry.size.toLocaleString()} bytes)`}
          </dd>
        </>
      )}
      <dt>Modified</dt>
      <dd>{formatDateTime(entry.modTime)}</dd>
      <dt>Permissions</dt>
      <dd className={styles.mono}>{formatMode(entry.mode, entry.isDir, isLink)}</dd>
      {isLink && (
        <>
          <dt>Link target</dt>
          <dd className={styles.mono}>{entry.link}</dd>
        </>
      )}
    </dl>
  );
}
