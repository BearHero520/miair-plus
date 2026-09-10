import fs from 'node:fs/promises';
import sharp from 'sharp';
const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><rect width="64" height="64" rx="18" fill="#217365"/><g fill="none" stroke="#e2efe6" stroke-width="4.5" stroke-linecap="round"><path d="M15 26 Q23 18 32 25 T49 24"/><path d="M15 39 Q23 31 32 38 T49 37"/></g></svg>`;
for (const [file,size] of [['../fpk/ICON.PNG',64],['../fpk/ICON_256.PNG',256],['../fpk/app/ui/images/icon_64.png',64],['../fpk/app/ui/images/icon_128.png',128],['../fpk/app/ui/images/icon_256.png',256],['public/logo.png',256],['public/favicon.png',64]]) {
 await fs.mkdir(file.slice(0,file.lastIndexOf('/')), {recursive:true});
 await sharp(Buffer.from(svg)).resize(size,size).png().toFile(file);
}
