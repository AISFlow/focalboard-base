// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.
import {promises as fs} from 'fs'
import * as path from 'path'

import winston from 'winston'

// Create a logger instance
const logger = winston.createLogger({
    level: 'info',
    transports: [
        new winston.transports.Console(),
    ],
})

// Define an interface for the language file contents
interface LangData {
    [key: string]: string
}

// Define the i18n directory relative to the current script (located beside package.json)
const i18nDir: string = path.join(__dirname, 'i18n')

// Define the reference file (en.json) path
const enFilePath: string = path.join(i18nDir, 'en.json')

/**
 * Reads and parses a JSON file.
 * @param filePath - The path to the JSON file.
 * @returns Parsed JSON data as LangData.
 */
async function readJsonFile(filePath: string): Promise<LangData> {
    try {
        const data = await fs.readFile(filePath, 'utf-8')
        return JSON.parse(data) as LangData
    } catch (error) {
        throw new Error(`Failed to read or parse ${filePath}: ${error}`)
    }
}

/**
 * Updates a language file by ensuring only keys from the reference (en.json) exist,
 * and sorts the keys based on the reference order.
 * @param fileName - The language file name.
 * @param enKeys - Keys from en.json in defined order.
 * @param enData - The reference language data from en.json.
 */
async function updateLanguageFile(fileName: string, enKeys: string[], enData: LangData): Promise<void> {
    const filePath = path.join(i18nDir, fileName)
    let langData: LangData = {}

    try {
        langData = await readJsonFile(filePath)
    } catch (error) {
        logger.warn(`${fileName} could not be read. A new file will be created.`)
    }

    let updated = false
    const updatedLangData: LangData = {}

    // For each key in en.json, if it exists in langData then use it, otherwise, add the reference value.
    // Extra keys in langData (not in en.json) are omitted.
    for (const key of enKeys) {
        if (Object.prototype.hasOwnProperty.call(langData, key)) {
            updatedLangData[key] = langData[key]
        } else {
            updatedLangData[key] = enData[key]
            updated = true
        }
    }

    // Write the updated data back to the file
    await fs.writeFile(filePath, JSON.stringify(updatedLangData, null, 2), 'utf-8')
    logger.info(`${fileName} has been updated. ${updated ? 'Missing keys added.' : 'No changes made.'}`)
}

/**
 * Orchestrates the update of all language files in the i18n directory.
 */
async function updateAllLanguageFiles(): Promise<void> {
    let enData: LangData
    try {
        enData = await readJsonFile(enFilePath)
    } catch (error) {
        throw new Error(`Error reading reference file: ${error}`)
    }

    const enKeys = Object.keys(enData)

    try {
        const files = await fs.readdir(i18nDir)
        const targetFiles = files.filter((file) => file.endsWith('.json') && file !== 'en.json')
        await Promise.all(targetFiles.map((file) => updateLanguageFile(file, enKeys, enData)))
    } catch (error) {
        throw new Error(`Error reading i18n directory: ${error}`)
    }
}

updateAllLanguageFiles().catch((error) => {
    logger.error('Unexpected error:', error)
    throw error
})
