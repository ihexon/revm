install_name_tool -id @rpath/libkrun.1.dylib libkrun.1.14.0.dylib
install_name_tool -change /opt/local/lib/libvirglrenderer.1.dylib @rpath/libvirglrenderer.1.dylib libkrun.1.14.0.dylib
install_name_tool -change /opt/homebrew/opt/libepoxy/lib/libepoxy.0.dylib @rpath/libepoxy.0.dylib libkrun.1.14.0.dylib
ln -s libkrun.1.14.0.dylib libkrun.1.dylib

# change id for libkrunfw
install_name_tool -id @rpath/libkrunfw.4.dylib libkrunfw.4.dylib

# change id molten-vk
install_name_tool -id @@HOMEBREW_PREFIX@@/opt/molten-vk-krunkit/lib/libMoltenVK.dylib libMoltenVK.dylib

# change id virglrenderer
install_name_tool -id @@HOMEBREW_PREFIX@@/opt/virglrenderer/lib/libvirglrenderer.1.dylib libvirglrenderer.1.dylib

install_name_tool -change @@HOMEBREW_PREFIX@@/opt/molten-vk-krunkit/lib/libMoltenVK.dylib libMoltenVK.dylib
install_name_tool -change @@HOMEBREW_PREFIX@@/opt/molten-vk-krunkit/lib/libMoltenVK.dylib libMoltenVK.dylib

