import './preview'; // must come first: installs a stand-in bridge when not running inside Wails
import React from 'react';
import {createRoot} from 'react-dom/client';
import './style.css';
import App from './App';

createRoot(document.getElementById('root')!).render(
    <React.StrictMode>
        <App/>
    </React.StrictMode>
);
