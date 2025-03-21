import { promises as fs } from 'fs';
import * as path from 'path';

// Define an interface for the language file contents
interface LangData {
  [key: string]: string;
}

// Define the i18n directory relative to the current script (located beside package.json)
const i18nDir: string = path.join(__dirname, 'i18n');
// Define the reference file (en.json) path
const enFilePath: string = path.join(i18nDir, 'en.json');

/**
 * Reads and parses a JSON file.
 * @param filePath - The path to the JSON file.
 * @returns Parsed JSON data as LangData.
 */
async function readJsonFile(filePath: string): Promise<LangData> {
  try {
    const data = await fs.readFile(filePath, 'utf-8');
    return JSON.parse(data) as LangData;
  } catch (error) {
    throw new Error(`Failed to read or parse ${filePath}: ${error}`);
  }
}

/**
 * Updates a language file by ensuring all keys from the reference (en.json) exist,
 * and sorts the keys based on the reference order followed by extra keys in alphabetical order.
 * @param fileName - The language file name.
 * @param enKeys - Keys from en.json in defined order.
 * @param enData - The reference language data from en.json.
 */
async function updateLanguageFile(fileName: string, enKeys: string[], enData: LangData): Promise<void> {
  const filePath = path.join(i18nDir, fileName);
  let langData: LangData = {};

  try {
    langData = await readJsonFile(filePath);
  } catch (error) {
    console.warn(`Warning: ${fileName} could not be read. A new file will be created.`);
  }

  let updated = false;
  // Ensure all keys from en.json exist in langData
  for (const key of enKeys) {
    if (!Object.prototype.hasOwnProperty.call(langData, key)) {
      langData[key] = enData[key];
      updated = true;
    }
  }

  // Sort keys based on en.json order; extra keys appended alphabetically
  const sortedLangData: LangData = {};
  for (const key of enKeys) {
    if (Object.prototype.hasOwnProperty.call(langData, key)) {
      sortedLangData[key] = langData[key];
    }
  }
  const extraKeys = Object.keys(langData)
                          .filter(key => !enKeys.includes(key))
                          .sort((a, b) => a.localeCompare(b));
  for (const key of extraKeys) {
    sortedLangData[key] = langData[key];
  }

  // Write the updated and sorted data back to the file
  await fs.writeFile(filePath, JSON.stringify(sortedLangData, null, 2), 'utf-8');
  console.log(`${fileName} has been updated and keys sorted.` + (updated ? "" : " (No missing keys)"));
}

/**
 * Orchestrates the update of all language files in the i18n directory.
 */
async function updateAllLanguageFiles(): Promise<void> {
  let enData: LangData;
  try {
    enData = await readJsonFile(enFilePath);
  } catch (error) {
    console.error('Error:', error);
    process.exit(1);
  }
  
  const enKeys = Object.keys(enData);

  try {
    const files = await fs.readdir(i18nDir);
    const targetFiles = files.filter(file => file.endsWith('.json') && file !== 'en.json');

    await Promise.all(targetFiles.map(file => updateLanguageFile(file, enKeys, enData)));
  } catch (error) {
    console.error('Error reading i18n directory:', error);
    process.exit(1);
  }
}

updateAllLanguageFiles().catch(error => {
  console.error('Unexpected error:', error);
  process.exit(1);
});
