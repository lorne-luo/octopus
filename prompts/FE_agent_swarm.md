调用Agent Swarm，帮我实现所有./docs/*-FE-*.md描述的功能

执行流程：
第一步：规划阶段
- 使用/planning-with-files这个skill做任务规划，所有计划存进./plans,完成后不要删除
- 针对每一个需求，先判断是否还与当前codebase兼容，不兼容的需求标注为incompatible并跳过实现
- 找出各个需求点的依赖关系，优先完成前置依赖
- 对每个需求点进行任务拆解，制定plan list

第二步：评审阶段
- 创建一个Review Agent，审查Todo List
- 指出遗漏点、风险点、优化建议
- 评审通过后才进入执行阶段

第三步：执行阶段
- 根据Plan List，创建多个执行Agent并行工作
- 每完成一个里程碑，做质量检查
- 如果卡壳，回到规划阶段重新规划
- git commit separately for each feature

第四步：验收阶段
- 所有任务完成后，做最终质量复核
- 确保所有交付物符合预期

期望输出：
- 前后端都能够成功build

质量要求（可选）：
- 代码注释详细，易于维护