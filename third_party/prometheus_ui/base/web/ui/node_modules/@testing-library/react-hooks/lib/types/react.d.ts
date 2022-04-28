import { ComponentType } from 'react';
import { RenderHookOptions as BaseRenderHookOptions, RenderHookResult, ServerRenderHookResult, Act, CleanupCallback } from '.';
export declare type WrapperComponent<TProps> = ComponentType<TProps>;
export declare type RendererOptions<TProps> = {
    wrapper?: WrapperComponent<TProps>;
};
export declare type RenderHookOptions<TProps> = BaseRenderHookOptions<TProps> & {
    wrapper?: WrapperComponent<TProps>;
};
export declare type ReactHooksRenderer = {
    renderHook: <TProps, TResult>(callback: (props: TProps) => TResult, options?: RenderHookOptions<TProps>) => RenderHookResult<TProps, TResult>;
    act: Act;
    cleanup: () => Promise<void>;
    addCleanup: (callback: CleanupCallback) => () => void;
    removeCleanup: (callback: CleanupCallback) => void;
    suppressErrorOutput: () => () => void;
};
export declare type ReactHooksServerRenderer = Omit<ReactHooksRenderer, 'renderHook'> & {
    renderHook: <TProps, TResult>(callback: (props: TProps) => TResult, options?: RenderHookOptions<TProps>) => ServerRenderHookResult<TProps, TResult>;
};
export * from '.';
