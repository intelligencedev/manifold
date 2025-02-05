import type { NodeProps, HandleProps, Position } from '@vue-flow/core';

// Define the shape of the input prop:
interface RunCmdNodeData {
  style: object;
  labelStyle: object;
  type: 'RunCmdNode';
  inputs: {
    command: string;
  };
  outputs: any;
  hasInputs: boolean;
  hasOutputs: boolean;
  inputHandleColor: string;
  inputHandleShape: string;
  handleColor: string;
  outputHandleShape: string;
  run?: (command: string, args?: string[]) => Promise<any>; // Add run function to interface here
}

// Define the props for RunCmdNode
interface RunCmdNodeProps extends NodeProps {
  id: string;
  data: RunCmdNodeData;
}

// Define the emits for RunCmdNode
interface RunCmdNodeEmits {
  (e: 'update:data', payload: { id: string; data: RunCmdNodeData }): void;
}

// Declare the component itself:
declare const RunCmdNode: import('vue').DefineComponent<
    RunCmdNodeProps,
    {},
    {},
    {},
    {},
    {},
    {},
    RunCmdNodeEmits
>;

export default RunCmdNode;