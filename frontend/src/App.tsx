import { useEffect } from 'react';
import styles from './App.module.css';
import { DialogHost } from './components/dialogs/DialogHost';
import { Pane } from './components/pane/Pane';
import { StatusBar } from './components/shell/StatusBar';
import { Titlebar } from './components/shell/Titlebar';
import { TransfersView } from './components/transfers/TransfersView';
import { startBackend } from './stores/bootstrap';
import { useUi } from './stores/ui';

function useGlobalShortcuts() {
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (!(event.metaKey || event.ctrlKey) || event.altKey) return;
      const ui = useUi.getState();
      if (ui.dialog !== null) return;
      switch (event.key) {
        case ',':
          event.preventDefault();
          ui.openDialog({ kind: 'settings' });
          break;
        case '1':
          event.preventDefault();
          ui.setTab('browser');
          break;
        case '2':
          event.preventDefault();
          ui.setTab('transfers');
          break;
        default:
          break;
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, []);
}

export default function App() {
  const tab = useUi((s) => s.tab);
  useEffect(() => startBackend(), []);
  useGlobalShortcuts();

  return (
    <div className={styles.app}>
      <Titlebar />
      <main className={styles.main}>
        <div className={styles.panes} hidden={tab !== 'browser'}>
          <Pane paneId="left" />
          <Pane paneId="right" />
        </div>
        <div className={styles.transfers} hidden={tab !== 'transfers'}>
          <TransfersView />
        </div>
      </main>
      <StatusBar />
      <DialogHost />
    </div>
  );
}
