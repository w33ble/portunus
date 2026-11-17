import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import App from '../src/App';
import { trackInputModality } from '../src/lib/inputModality';
import '../src/styles.css';

const root = document.getElementById('root');
if (!root) throw new Error('missing #root element');

document.documentElement.dataset['platform'] = 'mac';
trackInputModality();

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>
);
