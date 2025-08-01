install_name_tool -change /opt/local/lib/libvirglrenderer.1.dylib @rpath/libvirglrenderer.1.dylib libkrun.1.14.0.dylib  
install_name_tool -change /opt/homebrew/opt/libepoxy/lib/libepoxy.0.dylib @rpath/libepoxy.0.dylib  libkrun.1.14.0.dylib  
