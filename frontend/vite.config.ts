import {defineConfig, Plugin} from 'vite'
import react from '@vitejs/plugin-react'
import {ChildProcess, spawn, spawnSync} from 'node:child_process'
import {connect} from 'node:net'
import {tmpdir} from 'node:os'
import {join, resolve} from 'node:path'

const BACKEND = {host: '127.0.0.1', port: Number(process.env.VOLTLOAD_PORT ?? 7878)}

const listening = () => new Promise<boolean>(done => {
  const s = connect(BACKEND.port, BACKEND.host)
  s.once('connect', () => { s.destroy(); done(true) })
  s.once('error', () => done(false))
})

// In the browser the UI talks to the real Go backend (`voltload serve`) through the /api proxy below.
// This starts that backend with the dev server, so `npm run dev` / `wails dev` is one command.
// Set VOLTLOAD_NO_BACKEND=1 to skip it (for example when running `voltload serve` yourself),
// or VOLTLOAD_PORT to use another port.
function goBackend(): Plugin {
  // Kept on globalThis: Vite re-creates plugins when this file changes, but the backend should live
  // for the whole dev-server process, not be stopped and raced against its own replacement.
  const g = globalThis as {voltloadBackend?: ChildProcess}
  return {
    name: 'voltload-backend',
    apply: 'serve',
    async configureServer(server) {
      const log = server.config.logger
      if (process.env.VOLTLOAD_NO_BACKEND || (g.voltloadBackend && g.voltloadBackend.exitCode === null) || await listening()) return
      const root = resolve(__dirname, '..')
      const bin = join(tmpdir(), `voltload-dev-${BACKEND.port}${process.platform === 'win32' ? '.exe' : ''}`)
      log.info('[voltload] building the Go backend…')
      // build, then run the binary directly: killing `go run` would orphan the server it starts
      const build = spawnSync('go', ['build', '-o', bin, '.'], {cwd: root, stdio: 'inherit'})
      if (build.status !== 0) {
        log.error('[voltload] backend build failed; the UI will show "Cannot reach the VoltLoad backend"')
        return
      }
      const child = spawn(bin, ['serve', '--addr', `${BACKEND.host}:${BACKEND.port}`], {cwd: root, stdio: 'inherit'})
      g.voltloadBackend = child
      child.on('exit', code => {
        if (g.voltloadBackend === child) g.voltloadBackend = undefined
        if (code) log.error(`[voltload] backend exited (code ${code}); restart the dev server to start it again`)
      })
      const stop = () => child.kill('SIGINT') // the backend stops a running test and saves it first
      process.once('exit', stop)
      for (const sig of ['SIGINT', 'SIGTERM', 'SIGHUP'] as const) process.once(sig, () => { stop(); process.exit() })
    },
  }
}

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react(), goBackend()],
  server: {
    proxy: {'/api': {target: `http://${BACKEND.host}:${BACKEND.port}`, changeOrigin: true}},
  },
})
