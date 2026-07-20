/**
 * Client entry point. Mounts the React tree into the shell served by the BFF.
 */

import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';

import { App } from './App.js';
import './index.css';

const container = document.getElementById('root');

if (container === null) {
  // Fail loudly rather than silently rendering nothing: a missing root means
  // the served HTML shell and this bundle have diverged.
  throw new Error('Root element #root not found in document');
}

createRoot(container).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
