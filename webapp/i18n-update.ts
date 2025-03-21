// i18n-update.ts
import * as fs from 'fs';
import * as path from 'path';

// Define an interface for the language file contents
interface LangData {
  [key: string]: string;
}

// Define the i18n directory relative to the current script (located beside package.json)
const i18nDir: string = path.join(__dirname, 'i18n');
// Define the reference file (en.json) path
const enFilePath: string = path.join(i18nDir, 'en.json');

// Read and parse the en.json file
let enData: LangData;
try {
  enData = JSON.parse(fs.readFileSync(enFilePath, 'utf-8')) as LangData;
} catch (err) {
  console.error('Failed to read en.json:', err);
  process.exit(1);
}

// Obtain the keys from en.json in the defined order
const enKeys: string[] = Object.keys(enData);

// Read all JSON files in the i18n directory (excluding en.json) and update them accordingly
fs.readdir(i18nDir, (err, files: string[]) => {
  if (err) {
    console.error('Failed to read the i18n directory:', err);
    process.exit(1);
  }

  files.filter((file: string) => file.endsWith('.json') && file !== 'en.json')
       .forEach((fileName: string) => {
         const filePath: string = path.join(i18nDir, fileName);
         let langData: LangData = {};

         // Attempt to read and parse the target language file
         try {
           langData = JSON.parse(fs.readFileSync(filePath, 'utf-8')) as LangData;
         } catch (error) {
           console.warn(`There was an issue reading ${fileName}. A new file will be created.`);
         }

         let updated: boolean = false;
         // Ensure all keys from en.json exist in the target file
         enKeys.forEach((key: string) => {
           if (!Object.prototype.hasOwnProperty.call(langData, key)) {
             langData[key] = enData[key];
             updated = true;
           }
         });

         // Sort keys based on the order in en.json; extra keys are appended alphabetically
         const sortedLangData: LangData = {};
         enKeys.forEach((key: string) => {
           if (Object.prototype.hasOwnProperty.call(langData, key)) {
             sortedLangData[key] = langData[key];
           }
         });
         const extraKeys: string[] = Object.keys(langData)
                                        .filter((key: string) => !enKeys.includes(key))
                                        .sort((a: string, b: string) => a.localeCompare(b));
         extraKeys.forEach((key: string) => {
           sortedLangData[key] = langData[key];
         });

         // Write the updated and sorted data back to the language file
         fs.writeFileSync(filePath, JSON.stringify(sortedLangData, null, 2), 'utf-8');
         if (updated) {
           console.log(`${fileName} has been updated and keys sorted.`);
         } else {
           console.log(`${fileName} is already up-to-date; keys have been sorted.`);
         }
       });
});
