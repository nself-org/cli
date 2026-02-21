#!/usr/bin/env node

const { spawn } = require('child_process');
const path = require('path');
const fs = require('fs');

// Find the nself installation
function findNselfPath() {
  // Check if nself is in PATH
  const { execSync } = require('child_process');
  try {
    const nselfPath = execSync('which nself', { encoding: 'utf8' }).trim();
    if (fs.existsSync(nselfPath)) {
      return nselfPath;
    }
  } catch (e) {
    // Continue to other checks
  }

  // Check common installation paths
  const commonPaths = [
    '/usr/local/bin/nself',
    '/usr/bin/nself',
    '/opt/nself/bin/nself',
    path.join(process.env.HOME || '', '.local', 'bin', 'nself')
  ];

  for (const nselfPath of commonPaths) {
    if (fs.existsSync(nselfPath)) {
      return nselfPath;
    }
  }

  return null;
}

function main() {
  const nselfPath = findNselfPath();
  
  if (!nselfPath) {
    console.error('Error: nself not found. Please install nself first:');
    console.error('curl -fsSL https://raw.githubusercontent.com/nself-org/cli/main/install.sh | bash');
    process.exit(1);
  }

  // Execute nself with all arguments
  const child = spawn(nselfPath, process.argv.slice(2), {
    stdio: 'inherit',
    shell: true
  });

  child.on('exit', (code) => {
    process.exit(code || 0);
  });

  child.on('error', (err) => {
    console.error('Error executing nself:', err.message);
    process.exit(1);
  });
}

main();