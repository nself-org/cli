#!/usr/bin/env node

const { execSync } = require('child_process');
const fs = require('fs');
const path = require('path');

console.log('Installing nself CLI...');

function checkNselfInstalled() {
  try {
    execSync('which nself', { stdio: 'ignore' });
    return true;
  } catch (e) {
    return false;
  }
}

function installNself() {
  try {
    console.log('Downloading and installing nself...');
    
    if (process.platform === 'win32') {
      // Windows installation
      console.log('For Windows, please run in WSL or use PowerShell:');
      console.log('irm https://raw.githubusercontent.com/nself-org/cli/main/install.ps1 | iex');
      return;
    }
    
    // Unix-like systems
    execSync('curl -fsSL https://raw.githubusercontent.com/nself-org/cli/main/install.sh | bash', {
      stdio: 'inherit'
    });
    
    console.log('✅ nself installed successfully!');
  } catch (error) {
    console.error('❌ Failed to install nself:', error.message);
    console.error('Please install manually:');
    console.error('curl -fsSL https://raw.githubusercontent.com/nself-org/cli/main/install.sh | bash');
    process.exit(1);
  }
}

if (checkNselfInstalled()) {
  console.log('✅ nself is already installed');
} else {
  installNself();
}