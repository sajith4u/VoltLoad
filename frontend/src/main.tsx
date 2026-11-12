import './bridge'; // must come first: connects the UI to the Go backend when not inside the desktop window
import React from 'react';
import {createRoot} from 'react-dom/client';
import './style.css';
import App from './App';

createRoot(document.getElementById('root')!).render(
    <React.StrictMode>
        <App/>
    </React.StrictMode>
);
