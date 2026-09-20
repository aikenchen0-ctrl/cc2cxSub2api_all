declare const __OCC_GATEWAY__: boolean;

export function gatewayLoginRequired(): boolean {
  return typeof __OCC_GATEWAY__ === 'boolean' ? __OCC_GATEWAY__ : false;
}
