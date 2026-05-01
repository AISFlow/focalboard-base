const { merge } = require('webpack-merge');
const TerserPlugin = require('terser-webpack-plugin');
const webpack = require('webpack');

const commonModule = require('./webpack.common.js');
const makeCommonConfig = typeof commonModule === 'function' ? commonModule : commonModule.default;

const commonConfig = makeCommonConfig();

const config = merge(commonConfig, {
    mode: 'production',
    devtool: false,
    optimization: {
        minimize: true,
        minimizer: [
            new TerserPlugin({
                extractComments: false,
                terserOptions: {
                    output: { comments: false },
                    compress: { drop_console: true },
                },
            }),
        ],
    },
    performance: {
        hints: 'warning',
        maxAssetSize: 512000,
        maxEntrypointSize: 512000,
    },
    plugins: [
        new webpack.DefinePlugin({
            'process.env.NODE_ENV': JSON.stringify('production'),
        }),
    ],
});

module.exports = config;
