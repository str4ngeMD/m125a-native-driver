#!/usr/bin/env bash
# build_native.sh
# Standard compilation script for compiling HPLIP's hpcups driver natively on Apple Silicon
set -e

BREW_PREFIX="/opt/homebrew"
INCLUDES="-I. -Icommon -Iprnt/hpcups -Iio/hpmud -I$BREW_PREFIX/include"
LIBS="/opt/homebrew/lib/libjpeg.a -L$BREW_PREFIX/lib -lcups -lz"
FLAGS="-O2 -Wall -DDISABLE_IMAGEPROCESSOR -DCONFDIR=\"/private/etc/hp\""

echo "Compiling native ARM64 objects..."

clang++ $FLAGS $INCLUDES -c prnt/hpcups/HPCupsFilter.cpp -o HPCupsFilter.o
clang++ $FLAGS $INCLUDES -c prnt/hpcups/Encapsulator.cpp -o Encapsulator.o
clang++ $FLAGS $INCLUDES -c prnt/hpcups/EncapsulatorFactory.cpp -o EncapsulatorFactory.o
clang++ $FLAGS $INCLUDES -c prnt/hpcups/Job.cpp -o Job.o
clang++ $FLAGS $INCLUDES -c prnt/hpcups/Pipeline.cpp -o Pipeline.o
clang++ $FLAGS $INCLUDES -c prnt/hpcups/Processor.cpp -o Processor.o
clang++ $FLAGS $INCLUDES -c prnt/hpcups/RasterSender.cpp -o RasterSender.o
clang++ $FLAGS $INCLUDES -c prnt/hpcups/SystemServices.cpp -o SystemServices.o
clang++ $FLAGS $INCLUDES -c prnt/hpcups/HPCupsUtils.cpp -o HPCupsUtils.o
clang++ $FLAGS $INCLUDES -c prnt/hpcups/Hbpl1.cpp -o Hbpl1.o
clang++ $FLAGS $INCLUDES -c prnt/hpcups/Hbpl1_Wrapper.cpp -o Hbpl1_Wrapper.o
clang++ $FLAGS $INCLUDES -c prnt/hpcups/genPCLm.cpp -o genPCLm.o
clang++ $FLAGS $INCLUDES -c prnt/hpcups/dbuscomm.cpp -o dbuscomm.o
clang++ $FLAGS $INCLUDES -c prnt/hpcups/RunLenEncoding.cpp -o RunLenEncoding.o
clang++ $FLAGS $INCLUDES -c prnt/hpcups/genJPEGStrips.cpp -o genJPEGStrips.o

clang $FLAGS $INCLUDES -c common/utils.c -o utils.o

echo "Linking native ARM64 hpcups-native executable..."
clang++ *.o $LIBS -o hpcups-native

echo "Build complete."
file hpcups-native
