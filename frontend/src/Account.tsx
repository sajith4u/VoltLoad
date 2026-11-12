import {FormEvent, useEffect, useState} from 'react';
import {ChangePassword, Login as login} from '../wailsjs/go/gui/App';
import Icon from './Icons';

// Go error messages start lowercase; capitalise for display.
const message = (err: unknown) => {
    const m = String(err instanceof Error ? err.message : err).replace(/^Error: /, '');
    return m.charAt(0).toUpperCase() + m.slice(1);
};

/** The sign-in page. The first account is admin / admin; change it from the sidebar once you're in. */
export function Login({onLogin, notice}: { onLogin: (user: string) => void; notice: string }) {
    const [user, setUser] = useState('');
    const [pass, setPass] = useState('');
    const [error, setError] = useState(notice);
    const [busy, setBusy] = useState(false);

    async function submit(e: FormEvent) {
        e.preventDefault();
        setBusy(true);
        setError('');
        try {
            await login(user.trim(), pass);
            onLogin(user.trim());
        } catch (err) {
            setError(message(err));
            setBusy(false);
        }
    }

    return (
        <div className="login-page">
            <form className="login-card" onSubmit={submit}>
                <div className="login-brand"><span className="logo"><Icon name="bolt"/></span> VoltLoad</div>
                <h2>Sign in</h2>
                <p className="muted">Put your systems under pressure.</p>
                {error && <div className="error" role="alert">{error}</div>}
                <label>Username
                    <input value={user} onChange={e => setUser(e.target.value)} autoComplete="username" autoFocus required/>
                </label>
                <label>Password
                    <input type="password" value={pass} onChange={e => setPass(e.target.value)} autoComplete="current-password" required/>
                </label>
                <button className="primary" disabled={busy}>{busy ? 'Signing in…' : 'Sign in'}</button>
                <p className="muted login-hint">First time? Sign in with <b>admin</b> / <b>admin</b>, then change the password from the sidebar.</p>
            </form>
        </div>
    );
}

export function ChangePasswordDialog({user, onClose}: { user: string; onClose: () => void }) {
    const [oldPass, setOld] = useState('');
    const [newPass, setNew] = useState('');
    const [again, setAgain] = useState('');
    const [error, setError] = useState('');
    const [done, setDone] = useState(false);

    useEffect(() => {
        const esc = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
        window.addEventListener('keydown', esc);
        return () => window.removeEventListener('keydown', esc);
    }, [onClose]);

    async function submit(e: FormEvent) {
        e.preventDefault();
        if (newPass !== again) { setError('The new passwords do not match.'); return; }
        try {
            await ChangePassword(user, oldPass, newPass);
            setDone(true);
            setError('');
        } catch (err) {
            setError(message(err));
        }
    }

    return (
        <div className="overlay" onMouseDown={e => e.target === e.currentTarget && onClose()}>
            <form className="dialog" role="dialog" aria-modal="true" aria-label="Change password" onSubmit={submit}>
                <h3>Change password</h3>
                {error && <div className="error" role="alert">{error}</div>}
                {done ? <p className="good">Password changed.</p> : <>
                    <label>Current password
                        <input type="password" value={oldPass} onChange={e => setOld(e.target.value)} autoComplete="current-password" autoFocus required/>
                    </label>
                    <label>New password
                        <input type="password" value={newPass} onChange={e => setNew(e.target.value)} autoComplete="new-password" minLength={4} required/>
                    </label>
                    <label>Repeat new password
                        <input type="password" value={again} onChange={e => setAgain(e.target.value)} autoComplete="new-password" required/>
                    </label>
                </>}
                <div className="buttons">
                    {!done && <button className="primary">Change password</button>}
                    <button type="button" onClick={onClose}>{done ? 'Close' : 'Cancel'}</button>
                </div>
            </form>
        </div>
    );
}
