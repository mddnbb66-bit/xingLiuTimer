package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

//go:embed frontend/dist
var assets embed.FS

func main() {
	// 1. 初始化通知服务
	notifier := notifications.New()

	// 2. 初始化我们的 B站提醒服务
	biliSvc := NewBiliBreakService(notifier)

	// 3. 创建应用
	app := application.New(application.Options{
		Name:        "心流计时器",
		Description: "自动记录专注，适时提醒休息",
		Services: []application.Service{
			application.NewService(notifier), // 注册通知服务
			application.NewService(biliSvc),  // 注册我们的服务
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// 4. 创建主窗口 (并把窗口对象存下来)
	mainWindow := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "心流计时器",
		Width:  1120,
		Height: 800,
		// 使用深色背景避免加载时白屏闪烁
		BackgroundColour: application.NewRGB(11, 16, 32),
		URL:              "/",
	})

	// 🔥🔥🔥 5. 关键修复：把窗口传给服务，没有这行，标题调试和弹窗都无效！🔥🔥🔥
	biliSvc.SetMainWindow(mainWindow)

	// 6. 运行
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}