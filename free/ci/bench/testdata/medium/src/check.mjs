import { greet } from './greet.mjs';

if (greet('ci') !== 'hello ci') {
  throw new Error('greet is broken');
}
