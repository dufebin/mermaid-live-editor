import { spawn } from 'node:child_process';

const children = [
  spawn('make', ['-C', 'server-go', 'run'], { stdio: 'inherit' }),
  spawn('pnpm', ['dev'], { stdio: 'inherit' })
];

const shutdown = () => {
  for (const child of children) {
    child.kill();
  }
};

process.on('SIGINT', () => {
  shutdown();
  process.exit(0);
});
process.on('SIGTERM', () => {
  shutdown();
  process.exit(0);
});
