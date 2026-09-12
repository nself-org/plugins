/**
 * Jest config for @nself/feature-flags-client.
 *
 * The package has no jest.config previously — jest ran with zero
 * TypeScript transform and every test failed at the `import` statement.
 * ts-jest was already an installed devDependency but never wired up.
 *
 * Source uses NodeNext module resolution and imports its own sibling
 * modules with an explicit `.js` extension (e.g. `./index.js`) even though
 * the file on disk is `index.ts` — the standard NodeNext-ESM-style
 * convention. Jest's CommonJS resolver does not do that rewrite on its
 * own, so moduleNameMapper strips the `.js` suffix back off before
 * resolution, letting ts-jest's transform pick up the matching `.ts`
 * file.
 */

/** @type {import('jest').Config} */
module.exports = {
  preset: 'ts-jest',
  testEnvironment: 'node',
  moduleNameMapper: {
    '^(\\.{1,2}/.*)\\.js$': '$1',
  },
};
