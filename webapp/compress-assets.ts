// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {promises as fs} from 'fs'
import path from 'path'
import {exec} from 'child_process'
import {promisify} from 'util'
import {
    gzip,
    brotliCompress,
    constants as zlibConstants,
    ZlibOptions,
    BrotliOptions,
} from 'zlib'

import glob from 'glob'
import winston, {Logger} from 'winston'

// Promisified functions
const execAsync: (command: string) => Promise<{ stdout: string; stderr: string }> = promisify(exec)
const globAsync: (pattern: string, options?: glob.IOptions) => Promise<string[]> = promisify(glob)
const gzipAsync: (data: Buffer, options?: ZlibOptions) => Promise<Buffer> = promisify(gzip)
const brotliCompressAsync: (data: Buffer, options?: BrotliOptions) => Promise<Buffer> = promisify(brotliCompress)

// Logger configuration
const logger: Logger = winston.createLogger({
    level: 'info',
    format: winston.format.combine(
        winston.format.timestamp(),
        winston.format.printf(({timestamp, level, message}: winston.Logform.TransformableInfo): string =>
            `${timestamp} [${level.toUpperCase()}] ${message}`,
        ),
    ),
    transports: [new winston.transports.Console()],
})

// Compression options interface
interface CompressionOptions {
    gzipOptions?: ZlibOptions
    brotliOptions?: BrotliOptions
    zstdOptions?: string
}

/**
 * Compresses data using Gzip and Brotli.
 */
async function compressWithGzipAndBrotli(
    data: Buffer,
    filePath: string,
    options?: CompressionOptions,
): Promise<void> {
    try {
        const gzip: Buffer = await gzipAsync(data, options?.gzipOptions)
        await fs.writeFile(`${filePath}.gz`, gzip)

        const brotli: Buffer = await brotliCompressAsync(data, options?.brotliOptions)
        await fs.writeFile(`${filePath}.br`, brotli)

        logger.info(`Compressed (Gzip + Brotli): ${filePath}`)
    } catch (error: unknown) {
        logger.error(`Compression failed (.gz/.br): ${filePath} - ${(error as Error).message}`)
    }
}

/**
 * Compresses file using Zstd via CLI.
 */
async function compressWithZstd(
    filePath: string,
    options?: CompressionOptions,
): Promise<void> {
    try {
        const extraOptions: string = options?.zstdOptions ?? ''
        const cmd: string = `zstd -f -q ${extraOptions} "${filePath}" -o "${filePath}.zst"`
        await execAsync(cmd)
        logger.info(`Compressed (Zstd): ${filePath}`)
    } catch (error: unknown) {
        logger.error(`Zstd compression failed: ${filePath} - ${(error as Error).message}`)
    }
}

/**
 * Compresses a single file using all compression methods.
 */
async function compressFile(
    filePath: string,
    options?: CompressionOptions,
): Promise<void> {
    try {
        const data: Buffer = await fs.readFile(filePath)
        await Promise.all([
            compressWithGzipAndBrotli(data, filePath, options),
            compressWithZstd(filePath, options),
        ])
    } catch (error: unknown) {
        logger.error(`Failed to process file: ${filePath} - ${(error as Error).message}`)
    }
}

/**
 * Compresses all assets matched by glob pattern.
 */
async function compressAssets(
    globPattern: string,
    options?: CompressionOptions,
): Promise<void> {
    try {
        const files: string[] = await globAsync(globPattern, {nodir: true})
        const assets: string[] = files.filter((file: string): boolean =>
            /\.(js|css|html|eot|tiff|svg|woff2?|ttf)$/.test(file),
        )

        logger.info(`Found ${assets.length} asset(s) to compress.`)
        await Promise.all(assets.map((file: string): Promise<void> => compressFile(file, options)))
    } catch (error: unknown) {
        logger.error(`Asset compression error: ${(error as Error).message}`)
        throw error
    }
}

// Default compression settings
const defaultCompressionOptions: CompressionOptions = {
    gzipOptions: {level: 9},
    brotliOptions: {
        params: {
            [zlibConstants.BROTLI_PARAM_QUALITY]: 11,
        },
    },
    zstdOptions: '-22',
}

// Entry point
async function main(): Promise<void> {
    try {
        const outputDir: string = path.resolve(__dirname, 'pack', 'static')
        const pattern: string = path.join(outputDir, '**', '*')
        await compressAssets(pattern, defaultCompressionOptions)
    } catch (error: unknown) {
        logger.error(`Unexpected error: ${(error as Error).message}`)
        throw error
    }
}

main()
