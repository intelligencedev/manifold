// Minimal shim so TS stops complaining about missing types for 'dagre'.
// We only need it to be typed as any for our usage.
declare module 'dagre' {
  const dagre: any;
  export default dagre;
}

