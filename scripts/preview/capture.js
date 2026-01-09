const puppeteer = require('puppeteer');
const { execSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const FRAMES_DIR = path.join(__dirname, 'frames');
const PUBLIC_DIR = path.join(__dirname, '..', '..', 'public');
const OUTPUT_WEBP = path.join(PUBLIC_DIR, 'ui-demo.webp');
const OUTPUT_GIF = path.join(PUBLIC_DIR, 'ui-demo.gif');

async function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

// Human-like typing delay (variable speed)
function humanDelay() {
  // Random delay between 40-150ms, occasionally longer pauses
  const base = 40 + Math.random() * 80;
  const pause = Math.random() < 0.1 ? 200 : 0; // 10% chance of longer pause
  return Math.floor(base + pause);
}

let globalFrameNum = 0;
async function captureFrame(page) {
  const framePath = path.join(FRAMES_DIR, `frame_${String(globalFrameNum++).padStart(4, '0')}.png`);
  await page.screenshot({ path: framePath });
  return framePath;
}

async function captureFrames(page, count, delay = 100) {
  for (let i = 0; i < count; i++) {
    await captureFrame(page);
    await sleep(delay);
  }
}

// Type with human-like speed and capture frames
async function typeHuman(page, text) {
  for (const char of text) {
    await page.keyboard.type(char);
    const delay = humanDelay();
    await sleep(delay);
    // Capture frame every few characters
    if (Math.random() < 0.3) await captureFrame(page);
  }
  await captureFrame(page);
}

async function main() {
  // Clean up and create frames directory
  if (fs.existsSync(FRAMES_DIR)) {
    fs.rmSync(FRAMES_DIR, { recursive: true });
  }
  fs.mkdirSync(FRAMES_DIR);

  console.log('Launching browser...');
  const browser = await puppeteer.launch({
    headless: true,
    args: ['--no-sandbox', '--disable-setuid-sandbox']
  });

  const page = await browser.newPage();
  // Higher resolution with 2x scale for crisp output
  await page.setViewport({ width: 1280, height: 800, deviceScaleFactor: 2 });

  try {
    console.log('Navigating to ooo UI...');
    await page.goto('http://localhost:8800', { waitUntil: 'networkidle0', timeout: 10000 });
    await sleep(1200);

    // 1. Initial state - storage list
    console.log('1. Capturing initial storage list...');
    await captureFrames(page, 20, 120);

    // 2. Navigate to items/*
    console.log('2. Navigating to items/*...');
    await page.evaluate(() => {
      window.location.hash = '/storage/keys/live/' + encodeURIComponent('items/*');
    });
    await sleep(1200);
    await captureFrames(page, 18, 120);

    // 3. Click Push button to add new item
    console.log('3. Opening Push dialog...');
    await page.evaluate(() => {
      const buttons = document.querySelectorAll('button');
      for (const btn of buttons) {
        if (btn.textContent.includes('Push')) {
          btn.click();
          break;
        }
      }
    });
    await sleep(1000);
    await captureFrames(page, 12, 120);

    // 4. Switch editor to text mode for easier input
    console.log('4. Switching to text mode in editor...');
    await page.evaluate(() => {
      const textBtn = document.querySelector('.jse-button.jse-group-button.jse-first');
      if (textBtn) textBtn.click();
    });
    await sleep(600);
    await captureFrames(page, 10, 120);

    // 5. Type new item data in the text editor
    console.log('5. Entering new item data...');
    const cmContent = await page.$('.cm-content[contenteditable="true"]');
    if (cmContent) {
      await cmContent.click();
      await sleep(200);
      // Select all existing content
      await page.keyboard.down('Control');
      await page.keyboard.press('a');
      await page.keyboard.up('Control');
      await sleep(100);
      await captureFrame(page);
      
      // Type properly formatted JSON with human-like speed
      const jsonLines = [
        '{',
        '  "name": "New Product",',
        '  "description": "Created via UI",',
        '  "price": 29.99,',
        '  "inStock": true',
        '}'
      ];
      
      for (let i = 0; i < jsonLines.length; i++) {
        await typeHuman(page, jsonLines[i]);
        if (i < jsonLines.length - 1) {
          await page.keyboard.press('Enter');
          await sleep(80);
          await captureFrame(page);
        }
      }
      await sleep(300);
      await captureFrames(page, 8, 120);
    } else {
      console.log('  Warning: Could not find CodeMirror content area');
    }

    // 6. Click Push Data button
    console.log('6. Pushing new item...');
    await page.evaluate(() => {
      const buttons = document.querySelectorAll('button');
      for (const btn of buttons) {
        if (btn.textContent.includes('Push Data')) {
          btn.click();
          break;
        }
      }
    });
    await sleep(1200);
    await captureFrames(page, 15, 120);

    // 7. Click on the newly created item row to open edit modal
    console.log('7. Opening edit modal for new item...');
    // Click on the first data row (newest item) to open edit modal
    await page.evaluate(() => {
      // Find the first data row in tbody and click it
      const tbody = document.querySelector('tbody');
      if (tbody) {
        const firstRow = tbody.querySelector('tr');
        if (firstRow) firstRow.click();
      }
    });
    await sleep(1200);
    await captureFrames(page, 15, 120);

    // 8. Switch to text mode in edit modal
    console.log('8. Switching to text mode in edit modal...');
    await page.evaluate(() => {
      const textBtn = document.querySelector('.modal .jse-button.jse-group-button.jse-first, .jse-button.jse-group-button.jse-first');
      if (textBtn) textBtn.click();
    });
    await sleep(600);
    await captureFrames(page, 8, 120);

    // 9. Edit the item in the modal
    console.log('9. Editing item in modal...');
    const editCm = await page.$('.cm-content[contenteditable="true"]');
    if (editCm) {
      await editCm.click();
      await sleep(200);
      await page.keyboard.down('Control');
      await page.keyboard.press('a');
      await page.keyboard.up('Control');
      await sleep(100);
      await captureFrame(page);
      
      // Type updated JSON with human-like speed
      const updatedLines = [
        '{',
        '  "name": "Updated Product",',
        '  "description": "Modified via UI",',
        '  "price": 39.99,',
        '  "inStock": true,',
        '  "featured": true',
        '}'
      ];
      
      for (let i = 0; i < updatedLines.length; i++) {
        await typeHuman(page, updatedLines[i]);
        if (i < updatedLines.length - 1) {
          await page.keyboard.press('Enter');
          await sleep(80);
          await captureFrame(page);
        }
      }
      await sleep(300);
      await captureFrames(page, 8, 120);
    } else {
      console.log('  Warning: Could not find CodeMirror content area for edit');
    }

    // 10. Save changes in modal
    console.log('10. Saving changes...');
    await page.evaluate(() => {
      const buttons = document.querySelectorAll('button');
      for (const btn of buttons) {
        if (btn.textContent.includes('Save')) {
          btn.click();
          break;
        }
      }
    });
    await sleep(1200);
    await captureFrames(page, 15, 120);

    // 11. Navigate to users
    console.log('11. Navigating to users/*...');
    await page.evaluate(() => {
      window.location.hash = '/storage/keys/live/' + encodeURIComponent('users/*');
    });
    await sleep(1200);
    await captureFrames(page, 15, 120);

    // 12. View a user
    console.log('12. Viewing users/1...');
    await page.evaluate(() => {
      window.location.hash = '/storage/key/live/' + encodeURIComponent('users/1');
    });
    await sleep(1200);
    await captureFrames(page, 15, 120);

    // 13. Navigate to statistics multiglob
    console.log('13. Navigating to statistics/*/*/*...');
    await page.evaluate(() => {
      window.location.hash = '/storage/keys/live/' + encodeURIComponent('statistics/*/*/*');
    });
    await sleep(1200);
    await captureFrames(page, 15, 120);

    // 14. Back to storage list
    console.log('14. Back to storage list...');
    await page.evaluate(() => {
      window.location.hash = '/storage';
    });
    await sleep(1200);
    await captureFrames(page, 20, 120);

    console.log(`Captured ${globalFrameNum} frames`);

  } catch (err) {
    console.error('Error during capture:', err.message);
    console.error(err.stack);
  }

  await browser.close();

  // Convert frames to animated webp and gif with higher quality
  console.log('Converting to animated WebP and GIF...');
  try {
    // High quality WebP - scale down from 2x capture for crisp result
    execSync(`ffmpeg -y -framerate 10 -i ${FRAMES_DIR}/frame_%04d.png -vf "scale=1024:-1:flags=lanczos" -c:v libwebp -lossless 0 -compression_level 4 -q:v 85 -loop 0 -preset picture -an -vsync 0 ${OUTPUT_WEBP}`, {
      stdio: 'inherit'
    });
    console.log(`Created: ${OUTPUT_WEBP}`);

    // High quality GIF with better palette
    execSync(`ffmpeg -y -framerate 10 -i ${FRAMES_DIR}/frame_%04d.png -vf "scale=1024:-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=256:stats_mode=diff[p];[s1][p]paletteuse=dither=floyd_steinberg" -loop 0 ${OUTPUT_GIF}`, {
      stdio: 'inherit'
    });
    console.log(`Created: ${OUTPUT_GIF}`);

  } catch (err) {
    console.error('Error converting:', err.message);
  }

  // Clean up frames
  fs.rmSync(FRAMES_DIR, { recursive: true });
  console.log('Done!');
}

main().catch(console.error);
