interface BluetoothLEScanFilter {
  services?: BluetoothServiceUUID[];
  name?: string;
  namePrefix?: string;
  manufacturerData?: Array<{
    companyIdentifier: number;
    dataPrefix?: BufferSource;
    mask?: BufferSource;
  }>;
  serviceData?: Array<{
    service: BluetoothServiceUUID;
    dataPrefix?: BufferSource;
    mask?: BufferSource;
  }>;
}

type BluetoothServiceUUID = number | string;

interface BluetoothDevice extends EventTarget {
  readonly id: string;
  readonly name?: string;
  readonly gatt?: BluetoothRemoteGATTServer;
}

interface BluetoothRemoteGATTServer {
  readonly connected: boolean;
  readonly device: BluetoothDevice;
  connect(): Promise<BluetoothRemoteGATTServer>;
  disconnect(): void;
}

declare module "element-resize-detector" {
  namespace elementResizeDetectorMaker {
    interface Erd {
      listenTo(
        element: Element | Element[],
        callback: (element: Element) => void,
      ): void;
      removeListener(
        element: Element,
        callback: (element: Element) => void,
      ): void;
      removeAllListeners(element: Element): void;
      uninstall(element: Element): void;
    }

    interface Options {
      strategy?: "scroll" | "object";
      callOnAdd?: boolean;
      debug?: boolean;
    }
  }

  function elementResizeDetectorMaker(
    options?: elementResizeDetectorMaker.Options,
  ): elementResizeDetectorMaker.Erd;

  export default elementResizeDetectorMaker;
}
