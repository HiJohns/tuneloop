module.exports = {
  root: true,
  env: {
    browser: true,
    es2021: true,
    node: true,
  },
  extends: [
    'eslint:recommended',
    'plugin:react/recommended',
    'plugin:react-hooks/recommended',
  ],
  parserOptions: {
    ecmaVersion: 'latest',
    sourceType: 'module',
    ecmaFeatures: {
      jsx: true,
    },
  },
  settings: {
    react: { version: '18.2' },
  },
  rules: {
    'no-undef': 'off',
    'no-unused-vars': 'off',
    'react-hooks/set-state-in-effect': 'off',
    'react/react-in-jsx-scope': 'off',
    'react/prop-types': 'off',
  },
};