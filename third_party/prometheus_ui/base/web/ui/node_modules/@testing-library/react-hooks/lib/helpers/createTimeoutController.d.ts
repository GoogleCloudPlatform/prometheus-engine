import { WaitOptions } from '../types';
declare function createTimeoutController(timeout: WaitOptions['timeout']): {
    onTimeout(callback: () => void): void;
    wrap(promise: Promise<void>): Promise<void>;
    cancel(): void;
    timedOut: boolean;
};
export { createTimeoutController };
