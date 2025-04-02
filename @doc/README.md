# LiveKit Server 依赖关系图

本目录包含自动生成的LiveKit Server组件依赖关系图。

## 文件说明

- `InitializeServer.dot` - Graphviz dot格式的依赖关系定义文件
- `InitializeServer.svg` - 由dot文件生成的可视化SVG图形
- `InitializeServer.html` - 用于方便查看SVG图的HTML文件，包含缩放控制

## 查看方式

1. **HTML查看器**：打开`InitializeServer.html`文件，可以在浏览器中交互式查看依赖图
   - 支持放大、缩小功能
   - 可以在新标签中打开SVG以获得更好的体验

2. **直接查看SVG**：用浏览器打开`InitializeServer.svg`文件

3. **修改图表**：如需修改依赖关系图，可编辑`InitializeServer.dot`文件，然后使用以下命令重新生成SVG：
   ```
   dot -Tsvg InitializeServer.dot -o InitializeServer.svg
   ```

## 颜色说明

图表中使用不同的颜色对组件进行了分类：

- 蓝色：配置和输入参数
- 绿色：核心组件和消息总线
- 黄色：客户端和服务组件
- 紫色：安全和通知组件
- 红色：最终的LiveKit服务器

线条颜色表示不同类型的依赖关系，线条粗细表示重要性。

## 如何使用

这些依赖关系图有助于：

1. 了解LiveKit Server的整体架构
2. 分析组件之间的依赖关系
3. 在开发和修改代码时作为参考
4. 帮助新团队成员理解系统结构

## 如何生成其他函数的依赖图

如果需要生成其他函数的依赖关系图，可以参考`InitializeServer.dot`的结构，创建相应的dot文件，然后使用Graphviz工具生成图表。 