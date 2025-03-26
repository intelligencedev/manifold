import type { NodeProps } from '@vue-flow/core';

// Define the shape of the input prop:
interface ResponseNodeData {
    type: 'ResponseNode';
    labelStyle: {
        fontWeight: string;
    };
    hasInputs: boolean;
    hasOutputs: boolean;
    inputs: {
        response: string;
    };
    outputs: {};
    style: {
        border: string;
        borderRadius: string;
        backgroundColor: string;
        color: string;
        width: string;
        height: string;
    };
}


// Define the props for ResponseNode
interface ResponseNodeProps extends NodeProps {
    id: string;
    data: ResponseNodeData;
}


// Define the emits for ResponseNode
interface ResponseNodeEmits {
  (e: 'update:data', payload: { id: string; data: ResponseNodeData }): void;
    (e: 'disable-zoom'): void;
    (e: 'enable-zoom'): void;
    (e: 'resize', payload: any): void;
}

// Declare the component
declare const ResponseNode: import('vue').DefineComponent<
    ResponseNodeProps,
    {},
    {},
    {},
    {},
    {},
    {},
    ResponseNodeEmits
>;

export default ResponseNode;