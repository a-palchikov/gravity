/*
Copyright 2019 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

const path = require('path');
const MiniCssExtractPlugin = require("mini-css-extract-plugin");
const HtmlWebPackPlugin = require('html-webpack-plugin');
const fs = require('fs');

const ROOT_PATH = path.join(__dirname, '../');
const E_BASE_PATH = path.join(ROOT_PATH, 'src');
const TELEBASE_PATH = path.join(ROOT_PATH, 'oss-src');
const SHARED_BASE_PATH = path.join(ROOT_PATH, 'shared');
const FAVICON_PATH = path.join(TELEBASE_PATH, '/assets/favicon.ico');

const ESLintPlugin = require('eslint-webpack-plugin');

// TODO(dima): complete migration from eslit-loader to eslint-webpack-plugin
// See https://www.npmjs.com/package/eslint-webpack-plugin and https://laurieontech.com/posts/eslint-webpack-plugin/
const options = {
    extensions: [`js`, `jsx`],
    exclude: [
      `/node_modules/`,
      `/assets/`,
      `/*.json/`,
    ],
};

if (!fs.existsSync(TELEBASE_PATH)){
  throw Error('cannot find Gravity open source directory');
}

module.exports = {

  entry: {
    app: ['./src/boot.js'],
  },

  optimization: {
    splitChunks: {
      cacheGroups: {
        vendors: {
          chunks: "all",
          name: "vendor",
          test: /([\\/]node_modules[\\/])/,
          priority: -10
        }
      }
    }
  },

  output: {
    // used by loaders to generate various URLs within CSS, JS based off publicPath
    publicPath: '/web/app/',

    path: path.join(ROOT_PATH, 'dist/app'),

    /*
    * format of the output file names. [name] stands for 'entry' keys
    * defined in the 'entry' section
    **/
    filename: '[name].[hash].js',

    // chunk file name format
    chunkFilename: '[name].[chunkhash].js'
  },

  resolve: {
    // some vendor libraries expect below globals to be defined
    alias: {
      shared: path.join(ROOT_PATH, '/shared/'),
      app: TELEBASE_PATH,
      'oss-app': TELEBASE_PATH,
      'e-app': E_BASE_PATH,
      jQuery: 'jquery',
    },

    /*
    * root path to resolve js our modules, enables us to use absolute path.
    * For ex: require('./../../../config') can be replaced with require('app/config')
    **/
    modules: ['node_modules'],
    extensions: ['.js', '.jsx']
  },

  noParse: function(content) {
    return /xterm.js$/.test(content);
  },

  rules: {
    fonts: {
      test: /fonts\/(.)+\.(woff|woff2|ttf|eot|svg)/,
      loader: "url-loader",
      options: {
        limit: 102400, // 100kb
        name: '/assets/fonts/[name].[ext]',
      }
    },

    svg: {
      test: /\.svg$/,
      loader: 'svg-url-loader',
      options: {
        noquotes: true,
      },
      exclude: /node_modules/
    },

    css({ dev } = {}){
      var use = []
      if (dev) {
        use = ['style-loader', 'css-loader'];
      } else {
        use = [MiniCssExtractPlugin.loader, 'css-loader']
      }

      return {
        test: /\.(css)$/,
        use: use
      }
    },

    scss({ dev } = {})
    {
      var sassLoader = {
        loader: 'sass-loader',
        options: {
          outputStyle: "compressed",
          precision: 9
        } };

      var use = []
      if (dev) {
        use = ['style-loader', 'css-loader', sassLoader];
      } else {
        use = [MiniCssExtractPlugin.loader, 'css-loader', sassLoader]
      }

      return {
        test: /\.(scss)$/,
        use: use
      }
    },

    inlineStyle: {
      /*
      * loads CSS for the rest of the app by ignores vendor folder.
      **/
      test: /\.scss$/,
      use: ['style-loader', 'css-loader', 'sass-loader']
    },

    images: {
      test: /\.(png|jpg|gif)$/,
      loader: "url-loader",
      options: {
        limit: 10000,
        name: '/assets/img/img-[hash:6].[ext]',
      }
    },

    jsx: jsx,
  },

  plugins: {
    // builds index html page, the main entry point for application
    createIndexHtml() {
      return createHtmlPluginInstance({
        filename: '../index.html',
        favicon: FAVICON_PATH,
        title: '',
        inject: true,
        template: 'src/index.ejs'
      })
    },

    // extracts all vendor styles and puts them into separate css file
    extractAppCss() {
      return new MiniCssExtractPlugin({
        filename: "styles.[contenthash].css",
      })
    },

    lint() {
      return new ESLintPlugin(options)
    }
  }
};

function jsx(args){
  args = args || {};
  var emitWarning = false;
  if(args.withHot){
    emitWarning = true;
  }

  return {
    include: [SHARED_BASE_PATH, E_BASE_PATH, TELEBASE_PATH],
    test: /\.(js|jsx)$/,
    exclude: /(node_modules)|(assets)/,
    use: [
      {
        loader: 'babel-loader',
      }
    ]
  }
}

function createHtmlPluginInstance(cfg) {
  cfg.inject = true;
  return new HtmlWebPackPlugin(cfg)
}
