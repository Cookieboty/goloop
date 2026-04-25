"use client";

import { useEffect, useState } from "react";
import { useRouter, usePathname } from "next/navigation";
import { isLoggedIn } from "@/lib/api";

export function AuthGuard({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const [checked, setChecked] = useState(false);
  const [isAuthenticating, setIsAuthenticating] = useState(true);

  useEffect(() => {
    const normalizedPath = pathname.replace(/\/$/, "") || "/";
    
    // 登录页面直接显示
    if (normalizedPath === "/login") {
      setChecked(true);
      setIsAuthenticating(false);
      return;
    }
    
    // 检查登录状态
    const loggedIn = isLoggedIn();
    
    if (!loggedIn) {
      // 未登录，立即跳转到登录页
      router.replace("/login");
      return;
    }
    
    // 已登录，显示内容
    setChecked(true);
    setIsAuthenticating(false);
  }, [pathname, router]);

  // 如果是登录页，直接显示
  if (pathname === "/login" || pathname === "/login/") {
    return <>{children}</>;
  }

  // 认证中显示加载状态
  if (isAuthenticating || !checked) {
    return (
      <div className="flex items-center justify-center min-h-screen bg-gray-950">
        <div className="text-center">
          <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500 mb-4"></div>
          <div className="text-gray-400 text-sm">验证登录状态...</div>
        </div>
      </div>
    );
  }

  return <>{children}</>;
}
