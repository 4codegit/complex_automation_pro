import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App';
import './index.css';

// Theme boot: light is the default. A ?theme= URL param wins (handy for
// screenshots/tests), then the saved preference.
const themeParam = new URLSearchParams(window.location.search).get('theme');
const theme = themeParam === 'dark' || themeParam === 'light'
  ? themeParam
  : (localStorage.getItem('cap.theme') ?? 'light');
document.documentElement.dataset.theme = theme;

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
