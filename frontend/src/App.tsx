import styles from './App.module.css';

export default function App() {
  return (
    <div className={styles.app}>
      <header className={styles.titlebar}>Portunus</header>
      <main className={styles.panes}>
        <section className={styles.pane} aria-label="Local" />
        <section className={styles.pane} aria-label="Remote" />
      </main>
    </div>
  );
}
