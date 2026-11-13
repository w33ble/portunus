import '@wailsio/runtime';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import App from './App';
import { trackInputModality } from './lib/inputModality';
import { IS_MAC } from './lib/platform';
import './styles.css';

const root = document.getElementById('root');
if (!root) throw new Error('missing #root element');

document.documentElement.dataset['platform'] = IS_MAC ? 'mac' : 'other';
trackInputModality();

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>
);
