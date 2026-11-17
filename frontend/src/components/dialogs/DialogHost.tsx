import { createFolder, renameEntry } from '../../stores/fileOps';
import { usePrompts } from '../../stores/prompts';
import { useUi } from '../../stores/ui';
import { ConfirmDeleteDialog } from './ConfirmDeleteDialog';
import { ConflictDialog } from './ConflictDialog';
import { ConnectionsDialog } from './ConnectionsDialog';
import { ErrorDialog } from './ErrorDialog';
import { HostKeyDialog } from './HostKeyDialog';
import { NameDialog } from './NameDialog';
import { QuickConnectDialog } from './QuickConnectDialog';
import { SecretDialog } from './SecretDialog';
import { SettingsDialog } from './SettingsDialog';

function UiDialog() {
  const dialog = useUi((s) => s.dialog);
  const close = useUi((s) => s.closeDialog);
  if (dialog === null) return null;

  switch (dialog.kind) {
    case 'confirmDelete':
      return (
        <ConfirmDeleteDialog connId={dialog.connId} targets={dialog.targets} onClose={close} />
      );
    case 'newFolder':
      return (
        <NameDialog
          title="New folder"
          label="Name"
          confirmLabel="Create"
          initial=""
          onSubmit={(name) => createFolder(dialog.paneId, name)}
          onClose={close}
        />
      );
    case 'rename':
      return (
        <NameDialog
          title="Rename"
          label="New name"
          confirmLabel="Rename"
          initial={dialog.name}
          onSubmit={(name) => renameEntry(dialog.paneId, dialog.path, name)}
          onClose={close}
        />
      );
    case 'quickConnect':
      return <QuickConnectDialog paneId={dialog.paneId} onClose={close} />;
    case 'connections':
      return <ConnectionsDialog connId={dialog.connId} onClose={close} />;
    case 'settings':
      return <SettingsDialog onClose={close} />;
    case 'error':
      return (
        <ErrorDialog
          title={dialog.title}
          message={dialog.message}
          hint={dialog.hint}
          onClose={close}
        />
      );
    default: {
      const unreachable: never = dialog;
      return unreachable;
    }
  }
}

/** Backend prompts go one at a time; the next appears when the head is answered or dismissed. */
function PromptDialog() {
  const head = usePrompts((s) => s.queue[0]);
  if (head === undefined) return null;

  switch (head.kind) {
    case 'hostkey':
      return <HostKeyDialog key={head.id} prompt={head} />;
    case 'secret':
      return <SecretDialog key={head.id} prompt={head} />;
    case 'conflict':
      return <ConflictDialog key={head.id} prompt={head} />;
    default: {
      const unreachable: never = head;
      return unreachable;
    }
  }
}

export function DialogHost() {
  const dialog = useUi((s) => s.dialog);
  return (
    <>
      <UiDialog key={dialog === null ? 'none' : dialogKey(dialog)} />
      <PromptDialog />
    </>
  );
}

function dialogKey(dialog: NonNullable<ReturnType<typeof useUi.getState>['dialog']>): string {
  return dialog.kind === 'rename' ? `rename:${dialog.path}` : dialog.kind;
}
