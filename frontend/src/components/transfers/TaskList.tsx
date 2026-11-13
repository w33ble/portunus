import type { TaskView } from '../../api/types';
import { formatBytes } from '../../lib/format';
import { baseName } from '../../lib/paths';
import styles from './JobRow.module.css';

interface Props {
  heading: string;
  tasks: readonly TaskView[];
}

export function TaskList({ heading, tasks }: Props) {
  if (tasks.length === 0) return null;
  return (
    <section>
      <h3 className={styles.taskHeading}>{heading}</h3>
      <ul className={styles.tasks}>
        {tasks.map((task) => (
          <li key={task.id} className={styles.task}>
            <span className={styles.taskName} title={`${task.src} → ${task.dst}`}>
              {baseName(task.src)}
            </span>
            {task.error ? (
              <span className={styles.taskError}>{task.error}</span>
            ) : (
              <span className={styles.taskProgress}>
                {formatBytes(task.done)} / {formatBytes(task.size)}
              </span>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}
