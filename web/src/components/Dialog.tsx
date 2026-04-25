"use client";

import { createContext, useContext, useState, ReactNode } from "react";

interface DialogOptions {
  title?: string;
  message: string;
  type?: "alert" | "confirm";
  confirmText?: string;
  cancelText?: string;
  danger?: boolean;
}

interface DialogContextType {
  alert: (message: string, title?: string) => Promise<void>;
  confirm: (message: string, title?: string, danger?: boolean) => Promise<boolean>;
}

const DialogContext = createContext<DialogContextType | null>(null);

export function useDialog() {
  const context = useContext(DialogContext);
  if (!context) {
    throw new Error("useDialog must be used within a DialogProvider");
  }
  return context;
}

export function DialogProvider({ children }: { children: ReactNode }) {
  const [dialogState, setDialogState] = useState<{
    isOpen: boolean;
    options: DialogOptions;
    resolve: ((value: boolean) => void) | null;
  }>({
    isOpen: false,
    options: { message: "", type: "alert" },
    resolve: null,
  });

  const alert = (message: string, title?: string): Promise<void> => {
    return new Promise((resolve) => {
      setDialogState({
        isOpen: true,
        options: {
          title: title || "提示",
          message,
          type: "alert",
          confirmText: "确定",
        },
        resolve: () => {
          resolve();
          return true;
        },
      });
    });
  };

  const confirm = (message: string, title?: string, danger?: boolean): Promise<boolean> => {
    return new Promise((resolve) => {
      setDialogState({
        isOpen: true,
        options: {
          title: title || "确认",
          message,
          type: "confirm",
          confirmText: "确定",
          cancelText: "取消",
          danger,
        },
        resolve,
      });
    });
  };

  const handleClose = (result: boolean) => {
    if (dialogState.resolve) {
      dialogState.resolve(result);
    }
    setDialogState({
      isOpen: false,
      options: { message: "", type: "alert" },
      resolve: null,
    });
  };

  const { isOpen, options } = dialogState;

  return (
    <DialogContext.Provider value={{ alert, confirm }}>
      {children}
      
      {isOpen && (
        <div className="fixed inset-0 bg-black/70 backdrop-blur-sm flex items-center justify-center z-[100] p-4 animate-in fade-in duration-200">
          <div className="bg-gradient-to-br from-gray-800/95 to-gray-900/95 backdrop-blur-md rounded-2xl border border-gray-700/50 shadow-2xl max-w-md w-full animate-in zoom-in-95 duration-200">
            {/* 图标和标题 */}
            <div className="p-6 pb-4">
              <div className="flex items-start gap-4">
                <div className={`flex items-center justify-center w-12 h-12 rounded-full flex-shrink-0 ${
                  options.danger 
                    ? 'bg-red-500/20 border border-red-500/30' 
                    : options.type === 'confirm'
                    ? 'bg-blue-500/20 border border-blue-500/30'
                    : 'bg-green-500/20 border border-green-500/30'
                }`}>
                  {options.danger ? (
                    <svg className="w-6 h-6 text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                    </svg>
                  ) : options.type === 'confirm' ? (
                    <svg className="w-6 h-6 text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8.228 9c.549-1.165 2.03-2 3.772-2 2.21 0 4 1.343 4 3 0 1.4-1.278 2.575-3.006 2.907-.542.104-.994.54-.994 1.093m0 3h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                    </svg>
                  ) : (
                    <svg className="w-6 h-6 text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                    </svg>
                  )}
                </div>
                <div className="flex-1 min-w-0">
                  <h3 className="text-lg font-semibold text-white mb-2">{options.title}</h3>
                  <p className="text-sm text-gray-300 whitespace-pre-wrap">{options.message}</p>
                </div>
              </div>
            </div>

            {/* 按钮 */}
            <div className="p-6 pt-2 flex gap-3">
              {options.type === "confirm" && (
                <button
                  onClick={() => handleClose(false)}
                  className="flex-1 px-4 py-2.5 bg-gray-700/50 hover:bg-gray-700 text-gray-300 rounded-lg transition-all font-medium"
                  autoFocus
                >
                  {options.cancelText}
                </button>
              )}
              <button
                onClick={() => handleClose(true)}
                className={`flex-1 px-4 py-2.5 rounded-lg transition-all font-medium shadow-lg ${
                  options.danger
                    ? 'bg-gradient-to-r from-red-600 to-red-500 hover:from-red-500 hover:to-red-600 text-white shadow-red-500/20'
                    : options.type === 'confirm'
                    ? 'bg-gradient-to-r from-blue-600 to-blue-500 hover:from-blue-500 hover:to-blue-600 text-white shadow-blue-500/20'
                    : 'bg-gradient-to-r from-green-600 to-emerald-500 hover:from-green-500 hover:to-emerald-600 text-white shadow-green-500/20'
                }`}
                autoFocus={options.type === "alert"}
              >
                {options.confirmText}
              </button>
            </div>
          </div>
        </div>
      )}
    </DialogContext.Provider>
  );
}
